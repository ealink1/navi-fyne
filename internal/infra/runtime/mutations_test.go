package runtime

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
	_ "modernc.org/sqlite"
)

func TestAtomicChangesCommitRollbackAndKeepBoundValues(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	conn, err := database.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, "CREATE TABLE items(id INTEGER PRIMARY KEY, name TEXT UNIQUE)"); err != nil {
		t.Fatal(err)
	}
	d, _ := domain.Resolve("sqlite")
	client := &databaseClient{session: db.NewSQLConnStatementExecer(conn), descriptor: d}
	unsafeValue := "中文\\'; DROP TABLE items; --"
	rows, err := client.ApplyStatements(ctx, []domain.Statement{{SQL: "INSERT INTO items VALUES (?,?)", Args: []any{1, unsafeValue}, RequireOne: true}})
	if err != nil || rows != 1 {
		t.Fatal(rows, err)
	}
	var name string
	if err = conn.QueryRowContext(ctx, "SELECT name FROM items WHERE id=1").Scan(&name); err != nil || name != unsafeValue {
		t.Fatal(name, err)
	}
	// The second conflict must also undo the first successful statement.
	_, err = client.ApplyStatements(ctx, []domain.Statement{{SQL: "UPDATE items SET name=? WHERE id=?", Args: []any{"modified", 1}, RequireOne: true}, {SQL: "DELETE FROM items WHERE id=?", Args: []any{999}, RequireOne: true}})
	if err == nil {
		t.Fatal("missing optimistic conflict")
	}
	var rolledBack *domain.RolledBackError
	if !errors.As(err, &rolledBack) || errors.Is(err, domain.ErrUnknownOutcome) {
		t.Fatal("confirmed rollback was not distinguished from uncertain writes", err)
	}
	if err = conn.QueryRowContext(ctx, "SELECT name FROM items WHERE id=1").Scan(&name); err != nil || name != unsafeValue {
		t.Fatal("partial batch committed", name, err)
	}
	_, err = client.ApplyStatements(ctx, []domain.Statement{{SQL: "INSERT INTO items VALUES (?,?)", Args: []any{2, "first"}, RequireOne: true}, {SQL: "INSERT INTO items VALUES (?,?)", Args: []any{2, "duplicate"}, RequireOne: true}})
	if err == nil {
		t.Fatal("duplicate key accepted")
	}
	var count int
	if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM items").Scan(&count); err != nil || count != 1 {
		t.Fatal("constraint failure left partial insert", count, err)
	}
}
