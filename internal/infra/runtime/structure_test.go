package runtime

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/sqlworkbench"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

func TestSQLiteStructuredDesignerAppliesAndRollsBackDDL(t *testing.T) {
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
	if _, err = conn.ExecContext(ctx, "CREATE TABLE items(id INTEGER PRIMARY KEY,name TEXT)"); err != nil {
		t.Fatal(err)
	}
	d, _ := domain.Resolve("sqlite")
	client := &databaseClient{session: db.NewSQLConnStatementExecer(conn), descriptor: d}
	value := "'中文'"
	request := domain.StructureRequest{Object: domain.Object{Name: "items"}, Changes: []domain.StructureChange{{Kind: "addColumn", Column: connection.ColumnDefinition{Name: "tag", Type: "TEXT", Nullable: "YES", Default: &value, HasDefault: true}}, {Kind: "addIndex", Index: domain.NewIndex{Name: "ix_tag", Columns: []domain.IndexColumn{{Name: "tag"}}}}}}
	info := domain.TableInfo{Columns: []connection.ColumnDefinition{{Name: "id", Type: "INTEGER", Nullable: "NO", Key: "PRI"}, {Name: "name", Type: "TEXT", Nullable: "YES"}}}
	statements, err := sqlworkbench.BuildStructure("sqlite", request, info)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ApplyStructure(ctx, statements)
	if err != nil || result.Applied != 2 || !result.Atomic {
		t.Fatal(result, err)
	}
	if _, err = conn.ExecContext(ctx, "INSERT INTO items(id,name)VALUES(1,'original')"); err != nil {
		t.Fatal(err)
	}
	var tag string
	if err = conn.QueryRowContext(ctx, "SELECT tag FROM items WHERE id=1").Scan(&tag); err != nil || tag != "中文" {
		t.Fatal(tag, err)
	}
	result, err = client.ApplyStructure(ctx, []string{"ALTER TABLE items ADD COLUMN rollback_test TEXT", "CREATE INDEX broken ON missing_table(id)"})
	if err == nil || result.Applied != 0 || !result.Attempted {
		t.Fatal("partial DDL committed", result, err)
	}
	var count int
	if err = conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('items') WHERE name='rollback_test'").Scan(&count); err != nil || count != 0 {
		t.Fatal("DDL rollback failed", count, err)
	}
}
