package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
)

type fakeClient struct {
	calls   atomic.Int32
	active  atomic.Int32
	overlap atomic.Bool
	closed  atomic.Int32
	delay   time.Duration
}

func (f *fakeClient) Close() error                                             { f.closed.Add(1); return nil }
func (f *fakeClient) Scopes(context.Context) ([]string, error)                 { return []string{"main"}, nil }
func (f *fakeClient) Objects(context.Context, string) ([]domain.Object, error) { return nil, nil }
func (f *fakeClient) Schema(context.Context, string, string) (string, error)   { return "", nil }
func (f *fakeClient) Execute(ctx context.Context, e domain.Execution) ([]domain.Result, error) {
	f.calls.Add(1)
	if f.active.Add(1) > 1 {
		f.overlap.Store(true)
	}
	defer f.active.Add(-1)
	timer := time.NewTimer(f.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return []domain.Result{{RowsAffected: 1}}, nil
	}
}
func testEngine(t *testing.T, readOnly bool) (*Engine, domain.Profile, *fakeClient) {
	t.Helper()
	profiles := testProfiles(t)
	profile := saveProfile(t, profiles, "sqlite", readOnly)
	client := &fakeClient{delay: time.Millisecond}
	e := NewEngine(profiles)
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return client, nil }
	t.Cleanup(func() { _ = e.Close() })
	return e, profile, client
}
func TestEngineRequiresBoundSingleUseConfirmation(t *testing.T) {
	e, p, client := testEngine(t, false)
	ctx := context.Background()
	request := domain.Execution{Text: "DELETE FROM test", Write: true, Scope: "main"}
	_, err := e.Execute(ctx, p.ID, request)
	var required *domain.ConfirmationRequired
	if !errors.As(err, &required) {
		t.Fatal("missing confirmation")
	}
	if client.calls.Load() != 0 {
		t.Fatal("write executed before confirmation")
	}
	request.Confirmation = required.Fingerprint
	changed := request
	changed.Text = "DROP TABLE test"
	_, err = e.Execute(ctx, p.ID, changed)
	if !errors.As(err, &required) {
		t.Fatal("confirmation allowed changed SQL")
	}
	if client.calls.Load() != 0 {
		t.Fatal("changed write executed")
	}
	request.Confirmation = ""
	_, err = e.Execute(ctx, p.ID, request)
	if !errors.As(err, &required) {
		t.Fatal(err)
	}
	request.Confirmation = required.Fingerprint
	if _, err = e.Execute(ctx, p.ID, request); err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("write not executed once")
	}
	if _, err = e.Execute(ctx, p.ID, request); !errors.As(err, &required) {
		t.Fatal("confirmation was reusable")
	}
	if client.calls.Load() != 1 {
		t.Fatal("replayed confirmation executed")
	}
}
func TestEngineReadOnlyCannotBeBypassedByUIFlags(t *testing.T) {
	e, p, client := testEngine(t, true)
	_, err := e.Execute(context.Background(), p.ID, domain.Execution{Text: "DELETE FROM t", Write: true, Confirmation: "anything"})
	if !errors.Is(err, domain.ErrReadOnly) || client.calls.Load() != 0 {
		t.Fatal("read-only bypass")
	}
}
func TestEngineSerializesSessionsAndPreservesSQLiteScope(t *testing.T) {
	e, p, client := testEngine(t, true)
	client.delay = 20 * time.Millisecond
	var opens atomic.Int32
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { opens.Add(1); return client, nil }
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope := ""
			if i%2 == 0 {
				scope = "main"
			}
			_, err := e.Execute(context.Background(), p.ID, domain.Execution{Text: "SELECT 1", Scope: scope})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if client.overlap.Load() || opens.Load() != 1 {
		t.Fatal("SQLite scopes opened separate or overlapping sessions")
	}
}
func TestCanceledExecutionIsNotRetriedAndDiscardsTransport(t *testing.T) {
	e, p, client := testEngine(t, true)
	client.delay = time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := e.Execute(ctx, p.ID, domain.Execution{Text: "SELECT 1"})
	if !errors.Is(err, context.DeadlineExceeded) || client.calls.Load() != 1 || client.closed.Load() != 1 {
		t.Fatal("canceled request leaked or retried")
	}
}
