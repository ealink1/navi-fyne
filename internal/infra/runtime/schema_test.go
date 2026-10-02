package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

type sessionFixture struct {
	queries      []string
	execs        []string
	failCommit   bool
	failRollback bool
}

func (f *sessionFixture) Query(query string) ([]map[string]any, []string, error) {
	return f.QueryContext(context.Background(), query)
}
func (f *sessionFixture) QueryContext(_ context.Context, query string) ([]map[string]any, []string, error) {
	f.queries = append(f.queries, query)
	return []map[string]any{{"search_path": `"$user", public, "shared.schema"`}}, []string{"search_path"}, nil
}
func (f *sessionFixture) Exec(query string) (int64, error) {
	return f.ExecContext(context.Background(), query)
}
func (f *sessionFixture) ExecContext(_ context.Context, query string) (int64, error) {
	f.execs = append(f.execs, query)
	if query == "COMMIT" && f.failCommit || query == "ROLLBACK" && f.failRollback {
		return 0, errors.New("fixture connection lost")
	}
	return 1, nil
}
func (*sessionFixture) ExecContextWithArgs(context.Context, string, []any) (int64, error) {
	return 1, nil
}
func (*sessionFixture) Close() error { return nil }

func TestPostgresSchemaSelectionRestoresOriginalSearchPath(t *testing.T) {
	d, _ := domain.Resolve("postgres")
	f := &sessionFixture{}
	c := &databaseClient{descriptor: d, session: f}
	ctx := context.Background()
	if err := c.selectSchema(ctx, `tenant".schema`); err != nil {
		t.Fatal(err)
	}
	if len(f.execs) != 1 || f.execs[0] != `SET search_path TO "tenant"".schema"` {
		t.Fatal(f.execs)
	}
	if err := c.selectSchema(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if err := c.selectSchema(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 2 || !strings.Contains(f.queries[1], `"$user", public, "shared.schema"`) || c.schemaSelected {
		t.Fatal("session schema leaked", f.queries)
	}
	if err := c.selectSchema(ctx, ""); err != nil || len(f.queries) != 2 {
		t.Fatal("redundant search-path change", err)
	}
}
func TestAtomicWriterFlagsUncertainCommitOutcome(t *testing.T) {
	d, _ := domain.Resolve("sqlite")
	f := &sessionFixture{failCommit: true}
	c := &databaseClient{descriptor: d, session: f}
	_, err := c.ApplyStatements(context.Background(), []domain.Statement{{SQL: "INSERT INTO items VALUES(?)", Args: []any{1}, RequireOne: true}})
	if !errors.Is(err, domain.ErrUnknownOutcome) || f.execs[len(f.execs)-1] != "ROLLBACK" {
		t.Fatal("ambiguous commit became retryable", f.execs, err)
	}
}
