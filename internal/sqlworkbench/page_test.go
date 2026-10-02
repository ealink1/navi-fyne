package sqlworkbench

import (
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func testInfo() domain.TableInfo {
	return domain.TableInfo{Columns: []connection.ColumnDefinition{{Name: "id", Key: "PRI", Type: "INTEGER"}, {Name: "name", Type: "TEXT"}, {Name: "optional", Type: "TEXT", Nullable: "YES"}}}
}

func TestBuildPageQualifiesScopesAndKeepsValuesLiteral(t *testing.T) {
	request := domain.TableRequest{Object: domain.Object{Scope: "db.name", Name: "table`name"}, Page: 3, Size: 50, Filters: []domain.Filter{{Column: "name", Operator: "=", Value: "\\'; DROP TABLE x; --"}}}
	query, count, err := BuildPage("mysql", request, testInfo())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "`db.name`.`table``name`") || !strings.HasSuffix(query, "ORDER BY `id` ASC LIMIT 50 OFFSET 100") {
		t.Fatal(query)
	}
	if strings.Contains(query, "DROP TABLE") || !strings.Contains(query, "CONVERT(X'") || strings.Contains(count, "LIMIT") {
		t.Fatal("filter escaped SQL literal boundary", query, count)
	}
	request.Object = domain.Object{Schema: "a.b", Name: "c\"d"}
	request.Filters = nil
	query, _, err = BuildPage("postgres", request, testInfo())
	if err != nil || !strings.Contains(query, `"a.b"."c""d"`) {
		t.Fatal(query, err)
	}
}

func TestBuildPageRejectsInvalidMetadataAndPagination(t *testing.T) {
	base := domain.TableRequest{Object: domain.Object{Name: "items"}, Page: 1, Size: 100}
	cases := []domain.TableRequest{base, base, base, base, base}
	cases[0].Size = 0
	cases[1].Page = -1
	cases[2].Size = 1001
	cases[3].Filters = []domain.Filter{{Column: "missing", Operator: "="}}
	cases[4].Sorts = []domain.Sort{{Column: "id; DELETE"}}
	for i, request := range cases {
		if _, _, err := BuildPage("sqlite", request, testInfo()); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	base.Page = 2
	query, _, err := BuildPage("sqlserver", base, testInfo())
	if err != nil || !strings.HasSuffix(query, "OFFSET 100 ROWS FETCH NEXT 100 ROWS ONLY") {
		t.Fatal(query, err)
	}
}

func TestParsePathPreservesQuotedDotsAndEscapedQuotes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{{`public.items`, []string{"public", "items"}}, {`"a.b"."c""d"`, []string{"a.b", "c\"d"}}, {`"a.b"`, []string{"a.b"}}} {
		parts, err := ParsePath(tc.in)
		if err != nil || strings.Join(parts, "|") != strings.Join(tc.want, "|") {
			t.Fatal(tc.in, parts, err)
		}
	}
	for _, value := range []string{"", `a..b`, `"a`, "a.b.c"} {
		if _, err := ParsePath(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
