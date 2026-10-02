package sqlworkbench

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestChangesUseBoundValuesAndCompleteOptimisticLocator(t *testing.T) {
	original := map[string]any{"id": int64(1), "name": "before", "optional": nil}
	changes := domain.TableChanges{Object: domain.Object{Schema: "a.b", Name: "items"}, Rows: []domain.RowChange{{Kind: "update", Original: original, Values: map[string]any{"name": "'; DELETE FROM items; --"}}, {Kind: "delete", Original: original}}}
	statements, err := BuildChanges("postgres", changes, testInfo())
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 2 || strings.Contains(statements[0].SQL, "DELETE FROM items") {
		t.Fatal(statements)
	}
	if statements[0].SQL != `UPDATE "a.b"."items" SET "name" = $1 WHERE "id" = $2 AND "name" = $3 AND "optional" IS NULL` {
		t.Fatal(statements[0].SQL)
	}
	if !reflect.DeepEqual(statements[0].Args, []any{"'; DELETE FROM items; --", int64(1), "before"}) || !statements[0].RequireOne {
		t.Fatal(statements[0])
	}
	if !strings.Contains(statements[1].SQL, `"optional" IS NULL`) || strings.Contains(statements[1].SQL, "= NULL") {
		t.Fatal(statements[1])
	}
}

func TestChangesRejectUnknownFieldsMissingLocatorAndNoOps(t *testing.T) {
	info := testInfo()
	base := domain.TableChanges{Object: domain.Object{Name: "items"}}
	for _, row := range []domain.RowChange{{Kind: "update", Values: map[string]any{"name": "x"}}, {Kind: "insert", Values: map[string]any{"unknown": "x"}}, {Kind: "insert"}, {Kind: "drop"}} {
		base.Rows = []domain.RowChange{row}
		if _, err := BuildChanges("sqlite", base, info); err == nil {
			t.Fatal("unsafe row accepted", row)
		}
	}
	base.Rows = []domain.RowChange{{Kind: "delete", Original: map[string]any{"id": 1, "name": "x", "optional": nil}}}
	info.Columns[0].Key = ""
	if _, err := BuildChanges("sqlite", base, info); err == nil {
		t.Fatal("keyless delete accepted")
	}
	base.Rows = []domain.RowChange{{Kind: "insert", Values: map[string]any{"name": "x"}}}
	statements, err := BuildChanges("sqlite", base, info)
	if err != nil || statements[0].SQL != `INSERT INTO "items" ("name") VALUES (?)` {
		t.Fatal(statements, err)
	}
}
