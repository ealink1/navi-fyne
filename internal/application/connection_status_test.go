package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ealink1/navi-fyne/internal/domain"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
)

func assertConnectionStatus(t *testing.T, e *Engine, id string, want ConnectionStatus) {
	t.Helper()
	if got := e.ConnectionStatuses()[id]; got != want {
		t.Fatalf("connection status = %v, want %v", got, want)
	}
}

func TestConnectionStatusTracksFactoryAndDisconnect(t *testing.T) {
	e, p, client := testEngine(t, true)
	assertConnectionStatus(t, e, p.ID, ConnectionDisconnected)
	if err := e.Test(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionDisconnected)
	started, proceed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(proceed) }) })
	e.Factory = func(ctx context.Context, _ domain.Profile) (adapter.Client, error) {
		close(started)
		select {
		case <-proceed:
			return client, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	finished := make(chan error, 1) // A single worker can report without blocking test cleanup.
	go func() { _, err := e.Scopes(context.Background(), p.ID); finished <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connection did not start")
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnecting)
	select {
	case <-e.ConnectionChanges():
	default:
		t.Fatal("connection start did not notify the window")
	}
	once.Do(func() { close(proceed) })
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
	snapshot := e.ConnectionStatuses()
	snapshot[p.ID] = ConnectionFailed
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
	if err := e.Disconnect(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionDisconnected)
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if len(e.ConnectionStatuses()) != 0 {
		t.Fatal("closed engine exposes established sessions")
	}
}

func TestConnectionStatusFailureRetryAndDiscardedQuery(t *testing.T) {
	e, p, client := testEngine(t, true)
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) {
		return nil, errors.New("fixture cannot connect")
	}
	if _, err := e.Scopes(context.Background(), p.ID); err == nil {
		t.Fatal("failed factory was accepted")
	}
	assertConnectionStatus(t, e, p.ID, ConnectionFailed)
	interruptible := &statusCancellationClient{fakeClient: client}
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return interruptible, nil }
	if _, err := e.Scopes(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interruptible.cancel = cancel
	if _, err := e.Execute(ctx, p.ID, domain.Execution{Text: "SELECT 1"}); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled query did not fail", err)
	}
	if client.calls.Load() != 1 || client.closed.Load() != 1 {
		t.Fatal("cancellation did not reach execution and discard its transport")
	}
	assertConnectionStatus(t, e, p.ID, ConnectionFailed)
	interruptible.cancel = nil
	if _, err := e.Execute(context.Background(), p.ID, domain.Execution{Text: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
}

// Cancel only after the query reaches the driver, avoiding preflight timeouts.
type statusCancellationClient struct {
	*fakeClient
	cancel context.CancelFunc
}

func (c *statusCancellationClient) Execute(ctx context.Context, request domain.Execution) ([]domain.Result, error) {
	if c.cancel != nil {
		c.calls.Add(1)
		c.cancel()
		return nil, ctx.Err()
	}
	return c.fakeClient.Execute(ctx, request)
}

type deniedScopesClient struct{ *fakeClient }

func (*deniedScopesClient) Scopes(context.Context) ([]string, error) {
	return nil, errors.New("fixture metadata denied")
}

func TestConnectionStatusKeepsEstablishedSessionOnMetadataFailure(t *testing.T) {
	e, p, client := testEngine(t, true)
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return &deniedScopesClient{client}, nil }
	if _, err := e.Scopes(context.Background(), p.ID); err == nil {
		t.Fatal("metadata failure missing")
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
}

func TestConnectionStatusAggregatesScopesAndReadsDuringConcurrentOperations(t *testing.T) {
	e, p, _ := testEngine(t, true)
	p.Config.Type = "postgres"
	p, err := e.Profiles.Save(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	e.Factory = func(_ context.Context, profile domain.Profile) (adapter.Client, error) {
		if profile.Config.Database == "denied" {
			return nil, errors.New("fixture scope unavailable")
		}
		return &fakeClient{}, nil
	}
	if _, err := e.Objects(context.Background(), p.ID, "denied"); err == nil {
		t.Fatal("failed scope was accepted")
	}
	assertConnectionStatus(t, e, p.ID, ConnectionFailed)
	if _, err := e.Scopes(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	assertConnectionStatus(t, e, p.ID, ConnectionConnected)
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				e.ConnectionStatuses()
			}
		}()
	}
	if err := e.Disconnect(context.Background(), p.ID); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	assertConnectionStatus(t, e, p.ID, ConnectionDisconnected)
}
