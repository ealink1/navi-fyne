package runtime

import (
	"bytes"
	"database/sql"
	"math"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

func TestSQLiteBoundExecutionPreservesIntegerBinaryAndHostileText(t *testing.T) {
	ctx := t.Context()
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
	if _, err = conn.ExecContext(ctx, "CREATE TABLE items(id INTEGER PRIMARY KEY,body BLOB,name TEXT)"); err != nil {
		t.Fatal(err)
	}
	d, _ := domain.Resolve("sqlite")
	c := &databaseClient{session: db.NewSQLConnStatementExecer(conn), descriptor: d}
	wire, kinds, err := db.EncodeAgentArguments([]any{int64(math.MaxInt64), []byte{0, 255}, "'; DROP TABLE items; --"})
	if err != nil {
		t.Fatal(err)
	}
	args, err := db.DecodeAgentArguments(wire, kinds)
	if err != nil {
		t.Fatal(err)
	}
	results, err := c.Execute(ctx, domain.Execution{Text: "INSERT INTO items VALUES(?,?,?)", Args: args, Write: true})
	if err != nil || len(results) != 1 || results[0].RowsAffected != 1 {
		t.Fatal(results, err)
	}
	var id int64
	var blob []byte
	var name string
	if err = conn.QueryRowContext(ctx, "SELECT id,body,name FROM items").Scan(&id, &blob, &name); err != nil || id != math.MaxInt64 || !bytes.Equal(blob, []byte{0, 255}) || name != args[2] {
		t.Fatal(id, blob, name, err)
	}
	results, err = c.Execute(ctx, domain.Execution{Text: "SELECT id FROM items WHERE name=?", Args: []any{args[2]}})
	if err != nil || len(results) != 1 || len(results[0].Rows) != 1 {
		t.Fatal(results, err)
	}
}
