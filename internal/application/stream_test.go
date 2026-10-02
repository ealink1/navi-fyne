package application

import (
	"context"
	"errors"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
)

type streamFixture struct {
	*fakeClient
	rows int
}

func (f *streamFixture) StreamQuery(ctx context.Context, _ domain.Execution, c domain.RowConsumer) error {
	f.calls.Add(1)
	if err := c.SetColumns([]string{"id"}); err != nil {
		return err
	}
	for i := 0; i < f.rows; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.ConsumeRowValues([]any{int64(i)}); err != nil {
			return err
		}
	}
	return nil
}

type countFixture struct {
	rows   int
	failAt int
}

func (*countFixture) SetColumns([]string) error { return nil }
func (c *countFixture) ConsumeRowValues(row []any) error {
	if row[0] != int64(c.rows) {
		return errors.New("row order changed")
	}
	c.rows++
	if c.rows == c.failAt {
		return errors.New("fixture destination failed")
	}
	return nil
}
func TestStreamingExportPassesPreviewLimitAndClosesOnConsumerFailure(t *testing.T) {
	e, p, f := testEngine(t, true)
	stream := &streamFixture{fakeClient: f, rows: 10001}
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return stream, nil }
	consumer := &countFixture{}
	if err := e.StreamQuery(context.Background(), p.ID, domain.Execution{Text: "SELECT id FROM items"}, consumer); err != nil || consumer.rows != 10001 {
		t.Fatal(consumer.rows, err)
	}
	consumer = &countFixture{failAt: 2}
	if err := e.StreamQuery(context.Background(), p.ID, domain.Execution{Text: "SELECT id FROM items"}, consumer); err == nil || f.closed.Load() != 1 {
		t.Fatal("failed stream/session retained", err)
	}
}
func TestStreamingRejectsWritesAndMultipleResultsBeforeConnecting(t *testing.T) {
	e, p, _ := testEngine(t, false)
	calls := 0
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) {
		calls++
		return nil, errors.New("unexpected connection")
	}
	for _, sql := range []string{"DELETE FROM items", "SELECT 1; SELECT 2", "EXPLAIN ANALYZE DELETE FROM items"} {
		if err := e.StreamQuery(context.Background(), p.ID, domain.Execution{Text: sql}, &countFixture{}); err == nil {
			t.Fatal("unsafe export accepted", sql)
		}
	}
	if calls != 0 {
		t.Fatal("rejected query connected", calls)
	}
}
