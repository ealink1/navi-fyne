//go:build integration

package runtime

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

// Exercise the actual JSON-lines agent, binder and transaction, not an in-process substitute.
func TestLocalAgentsBoundValuesAndOptimisticChanges(t *testing.T) {
	for _, kind := range []string{"sqlite", "duckdb"} {
		t.Run(kind, func(t *testing.T) {
			installTestAgent(t, kind)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			client, err := Open(ctx, domain.Profile{Config: connection.ConnectionConfig{Type: kind, Host: filepath.Join(t.TempDir(), "bound.db"), Timeout: 5}})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			executeLive(t, ctx, client, "CREATE TABLE items(id BIGINT PRIMARY KEY, body BLOB, name VARCHAR(100))", true)
			text := "'; DROP TABLE items; -- 中文"
			if _, err := client.Execute(ctx, domain.Execution{Text: "INSERT INTO items VALUES(?,?,?)", Write: true, Args: []any{int64(math.MaxInt64), []byte{0, 255, 16}, text}}); err != nil {
				t.Fatal(err)
			}
			result := executeLive(t, ctx, client, "SELECT * FROM items", false)
			if len(result) != 1 || len(result[0].Rows) != 1 || fmt.Sprint(result[0].Rows[0][0]) != "9223372036854775807" || result[0].Rows[0][2] != text {
				t.Fatal("bound values changed during agent transport")
			}
			info, err := client.(TableInspector).TableInfo(ctx, "", "items")
			if err != nil {
				t.Fatal(err)
			}
			original := map[string]any{}
			for i, c := range result[0].Columns {
				original[c.Name] = result[0].Rows[0][i]
			}
			changes := domain.TableChanges{Object: domain.Object{Name: "items", Kind: "table"}, Rows: []domain.RowChange{{Kind: "update", Original: original, Values: map[string]any{"name": "changed"}}}}
			statements, err := sqlworkbench.BuildChanges(kind, changes, info)
			if err != nil {
				t.Fatal(err)
			}
			if n, err := client.(AtomicTableWriter).ApplyStatements(ctx, statements); err != nil || n != 1 {
				t.Fatalf("optimistic update of a binary-containing row: %d, %v", n, err)
			}
			result = executeLive(t, ctx, client, "SELECT hex(body),name FROM items", false)
			if fmt.Sprint(result[0].Rows[0][0]) != "00FF10" || result[0].Rows[0][1] != "changed" {
				t.Fatal("stored binary was damaged by a text-only edit")
			}
			testLiveRollback(t, ctx, client)
			query := "WITH RECURSIVE numbers(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM numbers WHERE x<10002) SELECT x FROM numbers WHERE x>?"
			if kind == "duckdb" {
				query = "SELECT range FROM range(10002) WHERE range>=?"
			}
			result, err = client.Execute(ctx, domain.Execution{Text: query, Args: []any{int64(0)}, MaxRows: 100})
			if err != nil || len(result) != 1 || len(result[0].Rows) != 100 || !result[0].Truncated {
				t.Fatal("parameter queries bypassed the agent's requested preview budget", err)
			}
		})
	}
}

func testLiveRollback(t *testing.T, ctx context.Context, client Client) {
	t.Helper()
	statements := []domain.Statement{
		{SQL: "UPDATE items SET name=?", Args: []any{"must rollback"}, RequireOne: true},
		{SQL: "INSERT INTO items(id,name) VALUES(?,?)", Args: []any{int64(math.MaxInt64), "duplicate"}, RequireOne: true},
	}
	_, err := client.(AtomicTableWriter).ApplyStatements(ctx, statements)
	var rolledBack *domain.RolledBackError
	if !errors.As(err, &rolledBack) || errors.Is(err, domain.ErrUnknownOutcome) {
		t.Fatal("constraint failure did not report its confirmed rollback", err)
	}
	result := executeLive(t, ctx, client, "SELECT hex(body),name FROM items", false)
	if result[0].Rows[0][1] != "changed" || fmt.Sprint(result[0].Rows[0][0]) != "00FF10" {
		t.Fatal("constraint rollback left partial row changes")
	}
}

func TestSQLiteAgentSessionMetadataDoesNotWaitForItsOwnPool(t *testing.T) {
	installTestAgent(t, "sqlite")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client, err := Open(ctx, domain.Profile{Config: connection.ConnectionConfig{Type: "sqlite", Host: ":memory:", Timeout: 5}})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	executeLive(t, ctx, client, "CREATE TABLE categories(id INTEGER PRIMARY KEY); CREATE TABLE items(id INTEGER PRIMARY KEY,category_id INTEGER REFERENCES categories(id),name TEXT,upper_name TEXT GENERATED ALWAYS AS (upper(name))); CREATE INDEX idx_items_name ON items(name); CREATE TRIGGER items_insert AFTER INSERT ON items BEGIN SELECT 1; END", true)
	info, err := client.(TableInspector).TableInfo(ctx, "main", "items")
	if err != nil || len(info.Warnings) != 0 || len(info.Columns) != 4 || len(info.Indexes) != 1 || len(info.ForeignKeys) != 1 || len(info.Triggers) != 1 {
		t.Fatalf("session metadata incomplete: %v, %v", info.Warnings, err)
	}
	if domain.WritableColumn(info.Columns[3]) || info.Indexes[0].ColumnName != "name" || info.ForeignKeys[0].RefTableName != "categories" || info.Triggers[0].Event != "INSERT" {
		t.Fatal("metadata identity or computed-column protection lost")
	}
}
