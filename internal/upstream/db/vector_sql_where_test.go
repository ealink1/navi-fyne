package db

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type vectorWhereRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn vectorWhereRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestVectorSQLWhereConversion(t *testing.T) {
	expr, found, err := parseVectorSQLWhere(`SELECT * FROM products WHERE (category = 'book' OR price < 5) AND active != false LIMIT 10;`)
	if err != nil || !found {
		t.Fatalf("parseVectorSQLWhere() = (%#v, %v, %v)", expr, found, err)
	}

	chromaJSON, _ := json.Marshal(chromaWhereFromExpr(expr))
	for _, fragment := range []string{`"$and"`, `"$or"`, `"category":{"$eq":"book"}`, `"price":{"$lt":5}`, `"active":{"$ne":false}`} {
		if !strings.Contains(string(chromaJSON), fragment) {
			t.Errorf("Chroma filter %s missing %s", chromaJSON, fragment)
		}
	}

	qdrantJSON, _ := json.Marshal(qdrantFilterFromExpr(expr))
	for _, fragment := range []string{`"must"`, `"should"`, `"key":"category"`, `"match":{"value":"book"}`, `"range":{"lt":5}`, `"must_not"`} {
		if !strings.Contains(string(qdrantJSON), fragment) {
			t.Errorf("Qdrant filter %s missing %s", qdrantJSON, fragment)
		}
	}
}

func TestVectorSQLWhereRejectsUnsupportedSyntax(t *testing.T) {
	queries := []string{
		`SELECT * FROM products WHERE category LIKE 'book%'`,
		`SELECT * FROM products WHERE price BETWEEN 1 AND 5`,
		`SELECT * FROM products WHERE id IN (1, 2)`,
		`SELECT * FROM products WHERE category = NULL`,
		`SELECT * FROM products WHERE (active = true`,
	}
	for _, query := range queries {
		if _, found, err := parseVectorSQLWhere(query); !found || err == nil {
			t.Errorf("parseVectorSQLWhere(%q) found=%v err=%v, want explicit error", query, found, err)
		}
	}
}

func TestVectorSQLWhereStopsBeforeOrderBy(t *testing.T) {
	expr, found, err := parseVectorSQLWhere(`SELECT * FROM products WHERE category = 'book' ORDER BY id LIMIT 10`)
	if err != nil || !found {
		t.Fatalf("parseVectorSQLWhere() = (%#v, %v, %v)", expr, found, err)
	}
	want := map[string]interface{}{"category": map[string]interface{}{"$eq": "book"}}
	if got := chromaWhereFromExpr(expr); !reflect.DeepEqual(got, want) {
		t.Fatalf("where = %#v, want %#v", got, want)
	}
}

func TestVectorSQLWhereDoesNotTreatOrderFieldAsOrderBy(t *testing.T) {
	expr, found, err := parseVectorSQLWhere(`SELECT * FROM products WHERE order = 3 LIMIT 10`)
	if err != nil || !found {
		t.Fatalf("parseVectorSQLWhere() = (%#v, %v, %v)", expr, found, err)
	}
	want := map[string]interface{}{"order": map[string]interface{}{"$eq": int64(3)}}
	if got := chromaWhereFromExpr(expr); !reflect.DeepEqual(got, want) {
		t.Fatalf("where = %#v, want %#v", got, want)
	}
}

func TestVectorSQLWhereAcceptsDoubledSingleQuote(t *testing.T) {
	expr, found, err := parseVectorSQLWhere(`SELECT * FROM products WHERE name = 'O''Reilly'`)
	if err != nil || !found {
		t.Fatalf("parseVectorSQLWhere() = (%#v, %v, %v)", expr, found, err)
	}
	want := map[string]interface{}{"name": map[string]interface{}{"$eq": "O'Reilly"}}
	if got := chromaWhereFromExpr(expr); !reflect.DeepEqual(got, want) {
		t.Fatalf("where = %#v, want %#v", got, want)
	}
}

func TestQdrantIDRangePredicateIsRejected(t *testing.T) {
	parsed, ok := parseQdrantSQL(`SELECT * FROM products WHERE id > 42`)
	if !ok || parsed.WhereError == nil || !strings.Contains(parsed.WhereError.Error(), "仅支持") {
		t.Fatalf("parseQdrantSQL() = (%#v, %v), want explicit point ID range error", parsed, ok)
	}
}

func TestChromaSQLIgnoresLiteralCountAndPagination(t *testing.T) {
	parsed, ok := parseChromaSQL(`SELECT * FROM products WHERE category = 'count('`)
	if !ok || parsed.Count || parsed.Collection != "products" || parsed.Limit != 200 {
		t.Fatalf("literal count( = %#v ok=%v, want record query", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT * FROM products WHERE category = 'LIMIT 1' LIMIT 20`)
	if !ok || parsed.Count || parsed.Limit != 20 || parsed.Offset != 0 {
		t.Fatalf("literal LIMIT = %#v ok=%v, want outer LIMIT 20", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT * FROM products WHERE category = 'OFFSET 9' LIMIT 20 OFFSET 5`)
	if !ok || parsed.Limit != 20 || parsed.Offset != 5 {
		t.Fatalf("literal OFFSET = %#v ok=%v, want outer OFFSET 5", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT * FROM products WHERE category = 'it\'s LIMIT 1 OFFSET 9' LIMIT 20 OFFSET 5`)
	if !ok || parsed.Count || parsed.Limit != 20 || parsed.Offset != 5 || parsed.WhereError != nil {
		t.Fatalf("backslash-escaped literal = %#v ok=%v, want outer pagination", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT 'it\'s COUNT(' FROM products`)
	if !ok || parsed.Count {
		t.Fatalf("backslash-escaped projection = %#v ok=%v, want record query", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT * FROM products WHERE name = 'O''LIMIT 1' LIMIT 20 OFFSET /* skip */ 3`)
	if !ok || parsed.Limit != 20 || parsed.Offset != 3 {
		t.Fatalf("escaped literal LIMIT = %#v ok=%v", parsed, ok)
	}

	parsed, ok = parseChromaSQL("SELECT * FROM products /* LIMIT 1 */ -- OFFSET 8\nLIMIT 20 OFFSET 4")
	if !ok || parsed.Limit != 20 || parsed.Offset != 4 {
		t.Fatalf("comment pagination = %#v ok=%v", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT COUNT (*) FROM "products" LIMIT 10 OFFSET 3`)
	if !ok || !parsed.Count || parsed.Collection != "products" || parsed.Limit != 10 || parsed.Offset != 3 {
		t.Fatalf("real COUNT = %#v ok=%v, want count with outer pagination", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT * FROM "LIMIT 1" OFFSET foo OFFSET 5 LIMIT 20`)
	if !ok || parsed.Collection != "LIMIT 1" || parsed.Limit != 20 || parsed.Offset != 5 {
		t.Fatalf("quoted table LIMIT = %#v ok=%v", parsed, ok)
	}

	parsed, ok = parseChromaSQL(`SELECT embedding FROM prod-ucts`)
	if !ok || parsed.Count || !parsed.IncludeEmbeddings || parsed.Collection != "prod-ucts" {
		t.Fatalf("embedding projection = %#v ok=%v", parsed, ok)
	}

	if parsed, ok := parseChromaSQL(`UPDATE products SET x = 1`); ok {
		t.Fatalf("non-select = %#v, want false", parsed)
	}
	if parsed, ok := parseChromaSQL(`SELECT 1`); ok {
		t.Fatalf("missing FROM = %#v, want false", parsed)
	}
	if parsed, ok := parseChromaSQL(`SELECT * FROM ""`); ok {
		t.Fatalf("empty quoted FROM = %#v, want false", parsed)
	}
}

func TestQdrantSQLIgnoresLiteralCountAndPagination(t *testing.T) {
	parsed, ok := parseQdrantSQL(`SELECT * FROM products WHERE category = 'count('`)
	if !ok || parsed.Count || parsed.Collection != "products" || parsed.Limit != 200 {
		t.Fatalf("literal count( = %#v ok=%v, want record query", parsed, ok)
	}

	parsed, ok = parseQdrantSQL(`SELECT * FROM products WHERE category = 'LIMIT 1' LIMIT 20 OFFSET point-2`)
	if !ok || parsed.Count || parsed.Limit != 20 || parsed.Offset != "point-2" {
		t.Fatalf("literal LIMIT/OFFSET = %#v ok=%v, want outer clauses", parsed, ok)
	}

	parsed, ok = parseQdrantSQL("SELECT * FROM `products` WHERE category = 'OFFSET 9' OFFSET /* cursor */ point.3")
	if !ok || parsed.Offset != "point.3" {
		t.Fatalf("comment OFFSET token = %#v ok=%v", parsed, ok)
	}

	parsed, ok = parseQdrantSQL(`SELECT * FROM products WHERE category = 'it\'s LIMIT 1 OFFSET wrong' LIMIT 20 OFFSET point-2`)
	if !ok || parsed.Count || parsed.Limit != 20 || parsed.Offset != "point-2" || parsed.WhereError != nil {
		t.Fatalf("backslash-escaped literal = %#v ok=%v, want outer pagination", parsed, ok)
	}

	parsed, ok = parseQdrantSQL(`SELECT 'it\'s COUNT(' FROM products`)
	if !ok || parsed.Count {
		t.Fatalf("backslash-escaped projection = %#v ok=%v, want record query", parsed, ok)
	}

	parsed, ok = parseQdrantSQL(`SELECT COUNT(*) FROM products`)
	if !ok || !parsed.Count {
		t.Fatalf("real COUNT = %#v ok=%v", parsed, ok)
	}

	parsed, ok = parseQdrantSQL(`SELECT id, vector FROM products LIMIT foo LIMIT 7 OFFSET = OFFSET 5`)
	if !ok || parsed.Limit != 7 || parsed.Offset != int64(5) {
		t.Fatalf("later real pagination = %#v ok=%v", parsed, ok)
	}

	if parsed, ok := parseQdrantSQL(`SHOW COLLECTIONS`); ok {
		t.Fatalf("non-select = %#v, want false", parsed)
	}
	if parsed, ok := parseQdrantSQL(`SELECT * FROM "unclosed`); ok {
		t.Fatalf("unclosed FROM quote = %#v, want false", parsed)
	}
}

func TestVectorWhereSQLLexingSkipsQuotesAndComments(t *testing.T) {
	if next, ok := skipSQLQuotedLiteral("", 0); ok || next != 0 {
		t.Fatalf("skip quoted empty = (%d, %v)", next, ok)
	}
	if next, ok := skipSQLQuotedLiteral("id", 0); ok || next != 0 {
		t.Fatalf("skip quoted ident = (%d, %v)", next, ok)
	}
	escapedLiteral := `'it\'s LIMIT 1'`
	if next, ok := skipSQLQuotedLiteral(escapedLiteral+" trailing", 0); !ok || next != len(escapedLiteral) {
		t.Fatalf("skip backslash-escaped literal = (%d, %v), want (%d, true)", next, ok, len(escapedLiteral))
	}
	if next, ok := skipSQLComment("", 0); ok || next != 0 {
		t.Fatalf("skip comment empty = (%d, %v)", next, ok)
	}
	if next, ok := skipSQLComment("-", 0); ok || next != 0 {
		t.Fatalf("skip comment dash = (%d, %v)", next, ok)
	}
	if next, ok := skipSQLComment("/", 0); ok || next != 0 {
		t.Fatalf("skip comment slash = (%d, %v)", next, ok)
	}
	if next, ok := skipSQLComment("id / 2", 3); ok {
		t.Fatalf("division must not skip as comment, next=%d", next)
	}

	if got := sqlSelectProjection("FROM products"); got != "" {
		t.Fatalf("projection without SELECT = %q", got)
	}
	if got := sqlSelectProjection("SELECT 1"); got != "" {
		t.Fatalf("projection without FROM = %q", got)
	}
	if got := parseSQLFromName("SELECT 1"); got != "" {
		t.Fatalf("FROM name without FROM = %q", got)
	}
	if got := parseSQLFromName("SELECT * FROM"); got != "" {
		t.Fatalf("FROM name without table = %q", got)
	}
	if got := parseSQLFromName("SELECT * FROM /* c */ products"); got != "products" {
		t.Fatalf("FROM after comment = %q", got)
	}
	if got := skipSQLSpaceAndComments("  -- gone", 0); got != len("  -- gone") {
		t.Fatalf("space+line comment skip = %d", got)
	}

	if sqlContainsFunctionCall(`'count(' /* count( */ accounting`, "COUNT") {
		t.Fatal("count inside quote/comment/identifier must not match")
	}
	if sqlContainsFunctionCall("COUNT", "COUNT") {
		t.Fatal("COUNT without parenthesis must not match")
	}
	if !sqlContainsFunctionCall("COUNT /* fn */ (*)", "COUNT") {
		t.Fatal("COUNT with comment before parenthesis must match")
	}

	if _, ok := parseSQLUnsignedInt(""); ok {
		t.Fatal("empty unsigned int must fail")
	}
	if _, ok := parseSQLUnsignedInt("x10"); ok {
		t.Fatal("non-digit unsigned int must fail")
	}
	if _, ok := parseSQLUnsignedInt("18446744073709551616"); ok {
		t.Fatal("overflow unsigned int must fail")
	}
	if n, ok := parseSQLUnsignedInt("20x"); !ok || n != 20 {
		t.Fatalf("leading digits = (%d, %v)", n, ok)
	}
	if _, ok := parseSQLLimitClause(`SELECT * FROM products WHERE category = 'LIMIT 1'`); ok {
		t.Fatal("LIMIT inside literal must be ignored")
	}
	if _, ok := parseSQLLimitClause("LIMIT"); ok {
		t.Fatal("LIMIT without value must be ignored")
	}
	if _, ok := parseSQLOffsetToken(`SELECT * FROM products OFFSET = 1`); ok {
		t.Fatal("OFFSET without token must be ignored")
	}
	if _, ok := parseSQLOffsetToken("OFFSET"); ok {
		t.Fatal("OFFSET without value must be ignored")
	}
	if idx := findSQLKeyword("SELECT * FROM products WHERE name = 'oops", "LIMIT", 0); idx >= 0 {
		t.Fatalf("unclosed quote still found LIMIT at %d", idx)
	}
	if idx := findSQLKeyword("SELECT * FROM products /* LIMIT 1", "LIMIT", 0); idx >= 0 {
		t.Fatalf("unclosed comment still found LIMIT at %d", idx)
	}
	if idx := findSQLKeyword("SELECT * FROM products WHERE x / 2 = 1 LIMIT 20", "LIMIT", 0); idx < 0 {
		t.Fatal("LIMIT after division must still be found")
	}
	if got := skipSQLSpaceAndComments("id", 0); got != 0 {
		t.Fatalf("no trivia skip = %d", got)
	}
}

func TestQdrantIDPredicatesUsePointIDFilter(t *testing.T) {
	tests := []struct {
		query string
		want  interface{}
	}{
		{`SELECT * FROM products WHERE id = 42`, map[string]interface{}{"must": []interface{}{map[string]interface{}{"has_id": []interface{}{int64(42)}}}}},
		{`SELECT * FROM products WHERE id != 'point-1'`, map[string]interface{}{"must_not": []interface{}{map[string]interface{}{"has_id": []interface{}{"point-1"}}}}},
	}
	for _, test := range tests {
		expr, _, err := parseVectorSQLWhere(test.query)
		if err != nil {
			t.Fatalf("parseVectorSQLWhere(%q): %v", test.query, err)
		}
		got := qdrantFilterFromExpr(expr)
		assertOfficialQdrantFilter(t, got)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("qdrantFilterFromExpr(%q) = %#v, want %#v", test.query, got, test.want)
		}
	}
}

func TestQdrantWhereSingleComparisonFiltersUseMust(t *testing.T) {
	tests := []struct {
		query string
		want  interface{}
	}{
		{
			`SELECT * FROM products WHERE category = 'book'`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "category", "match": map[string]interface{}{"value": "book"}}}},
		},
		{
			`SELECT * FROM products WHERE payload.price > 10`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "price", "range": map[string]interface{}{"gt": int64(10)}}}},
		},
		{
			`SELECT * FROM products WHERE price >= 10`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "price", "range": map[string]interface{}{"gte": int64(10)}}}},
		},
		{
			`SELECT * FROM products WHERE price < 5`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "price", "range": map[string]interface{}{"lt": int64(5)}}}},
		},
		{
			`SELECT * FROM products WHERE price <= 5`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "price", "range": map[string]interface{}{"lte": int64(5)}}}},
		},
		{
			`SELECT COUNT(*) FROM products WHERE id = 1`,
			map[string]interface{}{"must": []interface{}{map[string]interface{}{"has_id": []interface{}{int64(1)}}}},
		},
		{
			`SELECT * FROM products WHERE active != false`,
			map[string]interface{}{"must_not": []interface{}{map[string]interface{}{"key": "active", "match": map[string]interface{}{"value": false}}}},
		},
	}
	for _, test := range tests {
		expr, _, err := parseVectorSQLWhere(test.query)
		if err != nil {
			t.Fatalf("parseVectorSQLWhere(%q): %v", test.query, err)
		}
		got := qdrantFilterFromExpr(expr)
		assertOfficialQdrantFilter(t, got)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("qdrantFilterFromExpr(%q) = %#v, want %#v", test.query, got, test.want)
		}
	}
}

func TestQdrantWhereNestedMustShouldLogicIsPreserved(t *testing.T) {
	expr, found, err := parseVectorSQLWhere(`SELECT * FROM products WHERE (category = 'book' OR price < 5) AND active != false`)
	if err != nil || !found {
		t.Fatalf("parseVectorSQLWhere() = (%#v, %v, %v)", expr, found, err)
	}
	got := qdrantFilterFromExpr(expr)
	assertOfficialQdrantFilter(t, got)
	want := map[string]interface{}{
		"must": []interface{}{
			map[string]interface{}{
				"should": []interface{}{
					map[string]interface{}{"key": "category", "match": map[string]interface{}{"value": "book"}},
					map[string]interface{}{"key": "price", "range": map[string]interface{}{"lt": int64(5)}},
				},
			},
			map[string]interface{}{
				"must_not": []interface{}{
					map[string]interface{}{"key": "active", "match": map[string]interface{}{"value": false}},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("qdrantFilterFromExpr() = %#v, want %#v", got, want)
	}
}

func TestQdrantWhereScrollAndCountFiltersMatch(t *testing.T) {
	predicates := []string{
		`category = 'book'`,
		`price >= 10`,
		`id = 1`,
		`active != false`,
		`category = 'book' AND price < 5`,
		`active != false OR price < 5`,
	}
	for _, predicate := range predicates {
		scroll, scrollOK := parseQdrantSQL(`SELECT * FROM products WHERE ` + predicate)
		count, countOK := parseQdrantSQL(`SELECT COUNT(*) FROM products WHERE ` + predicate)
		if !scrollOK || !countOK || scroll.WhereError != nil || count.WhereError != nil {
			t.Fatalf("parseQdrantSQL(%q) scroll=(%#v, %v) count=(%#v, %v)", predicate, scroll, scrollOK, count, countOK)
		}
		assertOfficialQdrantFilter(t, scroll.Filter)
		assertOfficialQdrantFilter(t, count.Filter)
		if !reflect.DeepEqual(scroll.Filter, count.Filter) {
			t.Errorf("scroll/count filter mismatch for %q: scroll=%#v count=%#v", predicate, scroll.Filter, count.Filter)
		}
	}
}

func TestQdrantWhereFilterFromExprCoversNonConditionRoots(t *testing.T) {
	if got := qdrantFilterFromExpr(nil); got != nil {
		t.Fatalf("qdrantFilterFromExpr(nil) = %#v, want nil", got)
	}
	if got := qdrantEnsureRootFilter("not-a-filter"); got != "not-a-filter" {
		t.Fatalf("qdrantEnsureRootFilter(non-map) = %#v", got)
	}
	already := map[string]interface{}{"should": []interface{}{map[string]interface{}{"key": "category", "match": map[string]interface{}{"value": "book"}}}}
	if got := qdrantEnsureRootFilter(already); !reflect.DeepEqual(got, already) {
		t.Fatalf("qdrantEnsureRootFilter(should) = %#v, want original filter", got)
	}
	got := qdrantFilterFromExpr(vectorWhereComparison{Field: "ID", Op: ">", Value: int64(42)})
	want := map[string]interface{}{"must": []interface{}{map[string]interface{}{"key": "ID", "range": map[string]interface{}{"gt": int64(42)}}}}
	assertOfficialQdrantFilter(t, got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("id range comparison = %#v, want %#v", got, want)
	}
}

func assertOfficialQdrantFilter(t *testing.T, filter interface{}) {
	t.Helper()
	m, ok := filter.(map[string]interface{})
	if !ok {
		t.Fatalf("filter is not an object: %#v", filter)
	}
	allowed := map[string]struct{}{"must": {}, "should": {}, "must_not": {}, "min_should": {}}
	if len(m) == 0 {
		t.Fatalf("official Filter must not be empty: %#v", filter)
	}
	for key := range m {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("illegal Filter root key %q in %#v", key, filter)
		}
	}
}
