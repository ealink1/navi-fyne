package runtime

import (
	"context"
	"errors"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

type QueryStreamer interface {
	StreamQuery(context.Context, domain.Execution, domain.RowConsumer) error
}

func (c *databaseClient) StreamQuery(ctx context.Context, request domain.Execution, consumer domain.RowConsumer) error {
	if err := c.selectSchema(ctx, request.Schema); err != nil {
		return err
	}
	bridge := &streamBridge{consumer: consumer}
	if stream, ok := c.session.(db.StreamQueryExecer); ok {
		return stream.StreamQueryContext(ctx, request.Text, bridge)
	}
	if stream, ok := c.database.(db.StreamQueryExecer); ok {
		return stream.StreamQueryContext(ctx, request.Text, bridge)
	}
	return errors.New("driver does not support streaming export")
}
func (c *routedClient) StreamQuery(ctx context.Context, request domain.Execution, consumer domain.RowConsumer) error {
	if stream, ok := c.Client.(QueryStreamer); ok {
		return stream.StreamQuery(ctx, request, consumer)
	}
	return errors.New("driver does not support streaming export")
}

type streamBridge struct {
	consumer domain.RowConsumer
	columns  []string
}

func (s *streamBridge) SetColumns(columns []string) error {
	s.columns = append([]string(nil), columns...)
	return s.consumer.SetColumns(columns)
}
func (s *streamBridge) ConsumeRowValues(row []any) error { return s.consumer.ConsumeRowValues(row) }
func (s *streamBridge) ConsumeRow(row map[string]any) error {
	values := make([]any, len(s.columns))
	for i, c := range s.columns {
		values[i] = row[c]
	}
	return s.consumer.ConsumeRowValues(values)
}
