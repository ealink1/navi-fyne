package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
	"github.com/ealink1/super-link/internal/upstream/sqlaudit"
)

// StreamQuery exports one read query with the same revision and session guards.
func (e *Engine) StreamQuery(ctx context.Context, id string, request domain.Execution, consumer domain.RowConsumer) error {
	p, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return err
	}
	if request.Revision != 0 && request.Revision != p.Revision {
		return domain.ErrConflict
	}
	request, err = prepareExecution(p, request)
	if err != nil {
		return err
	}
	if len(request.Args) > 0 {
		return errors.New("parameterized queries currently export loaded results; streaming parameters are unsupported")
	}
	descriptor, err := domain.Resolve(p.Config.Type)
	if err != nil {
		return err
	}
	if descriptor.Family == domain.SQL {
		descriptor.Key = p.SQLDialect()
	}
	write, err := Classify(descriptor, request)
	if err != nil {
		return err
	}
	if write || request.Write {
		return errors.New("export query must be read-only")
	}
	tokens, err := sqlTokens(request.Text, p.SQLDialect())
	if err != nil {
		return err
	}
	for i, token := range tokens {
		if token == ";" && i != len(tokens)-1 {
			return errors.New("export accepts one result statement; select a statement before exporting")
		}
	}
	if len(tokens) == 0 || tokens[0] != "SELECT" && tokens[0] != "WITH" && tokens[0] != "SHOW" && tokens[0] != "EXPLAIN" {
		return errors.New("export requires a read query")
	}
	current, session, release, err := e.acquire(ctx, id, request.Scope)
	if err != nil {
		return err
	}
	defer release()
	if current.Revision != p.Revision {
		return domain.ErrConflict
	}
	if p.Config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.Config.QueryTimeout)*time.Second)
		defer cancel()
	}
	stream, ok := session.client.(adapter.QueryStreamer)
	if !ok {
		return errors.New("driver does not support streaming export")
	}
	start := time.Now()
	count := &countConsumer{RowConsumer: consumer}
	err = stream.StreamQuery(ctx, request, count)
	if err != nil {
		_ = session.client.Close()
		session.client = nil
		session.setConnectionStatus(ConnectionFailed)
	}
	if err == nil {
		err = e.checkRevision(ctx, id, p.Revision)
	}
	auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	auditErr := e.Profiles.Store.AddHistory(auditCtx, domain.History{ProfileID: id, Text: "EXPORT " + sqlaudit.RedactQuery(descriptor.Key, strings.TrimSpace(request.Text)), Success: err == nil, Rows: count.rows, Duration: time.Since(start)})
	if auditErr != nil && err == nil {
		return fmt.Errorf("export data read, but audit could not be saved: %w", auditErr)
	}
	return err
}

type countConsumer struct {
	domain.RowConsumer
	rows int64
}

func (c *countConsumer) ConsumeRowValues(row []any) error {
	if err := c.RowConsumer.ConsumeRowValues(row); err != nil {
		return err
	}
	c.rows++
	return nil
}
