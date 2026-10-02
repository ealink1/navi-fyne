package sqlworkbench

import (
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func structureFixture() domain.TableInfo {
	return domain.TableInfo{Columns: []connection.ColumnDefinition{{Name: "id", Type: "INTEGER", Key: "PRI", Nullable: "NO"}, {Name: "name", Type: "TEXT", Nullable: "YES"}}}
}
func TestStructuredDDLQuotesNamesAndUsesDialectIndexGrammar(t *testing.T) {
	for _, kind := range []string{"mysql", "postgres", "sqlite"} {
		r := domain.StructureRequest{Object: domain.Object{Name: "order.items", Schema: "tenant"}, Changes: []domain.StructureChange{{Kind: "alterColumn", OriginalName: "name", Column: connection.ColumnDefinition{Name: "renamed.name", Type: "TEXT", Nullable: "YES"}}, {Kind: "addIndex", Index: domain.NewIndex{Name: "ix.name", Method: "BTREE", Columns: []domain.IndexColumn{{Name: "renamed.name", Descending: true}}}}}}
		sql, err := BuildStructure(kind, r, structureFixture())
		if err != nil {
			t.Fatal(kind, err)
		}
		joined := strings.Join(sql, "\n")
		if !strings.Contains(joined, "renamed.name") || strings.Contains(joined, "ON `tenant`.`order.items` USING BTREE") {
			t.Fatal("invalid MySQL CREATE INDEX grammar", joined)
		}
		if kind == "mysql" && !strings.HasSuffix(sql[1], "USING BTREE") {
			t.Fatal(sql)
		}
		if kind == "postgres" && !strings.Contains(sql[1], "USING BTREE (") {
			t.Fatal(sql)
		}
	}
}
func TestStructuredDDLRejectsStaleColumnsInjectedTypesAndUnsupportedEdits(t *testing.T) {
	for _, change := range []domain.StructureChange{
		{Kind: "dropColumn", OriginalName: "missing"},
		{Kind: "dropColumn", OriginalName: "id"},
		{Kind: "addColumn", Column: connection.ColumnDefinition{Name: "a", Type: "TEXT); DROP TABLE items; --", Nullable: "YES"}},
		{Kind: "addColumn", Column: connection.ColumnDefinition{Name: "name", Type: "TEXT", Nullable: "YES"}},
		{Kind: "addIndex", Index: domain.NewIndex{Name: "ix", Columns: []domain.IndexColumn{{Name: "missing"}}}},
	} {
		if _, err := BuildStructure("postgres", domain.StructureRequest{Object: domain.Object{Name: "items"}, Changes: []domain.StructureChange{change}}, structureFixture()); err == nil {
			t.Fatal("invalid DDL allowed", change)
		}
	}
	if _, err := BuildStructure("sqlite", domain.StructureRequest{Object: domain.Object{Name: "items"}, Changes: []domain.StructureChange{{Kind: "alterColumn", OriginalName: "name", Column: connection.ColumnDefinition{Name: "name", Type: "INTEGER", Nullable: "YES"}}}}, structureFixture()); err == nil {
		t.Fatal("SQLite destructive rebuild silently approximated")
	}
	for _, value := range []string{"0); DROP TABLE items; --", "NOW(); DELETE FROM items", "'unclosed"} {
		if _, err := defaultExpression("postgres", value); err == nil {
			t.Fatal("unsafe default", value)
		}
	}
}
