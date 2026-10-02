package ui

import (
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestNamedQueriesOpenIndependentDraftsWithScopeAndNoExecution(t *testing.T) {
	w, p := parityWindow(t)
	query, err := w.Store.SaveQuery(t.Context(), domain.SavedQuery{ID: "query", ProfileID: p.ID, Title: "常用查询", Scope: "main", Schema: "public", Text: "SELECT 42;"})
	if err != nil {
		t.Fatal(err)
	}
	a, b := w.openSavedQuery(query), w.openSavedQuery(query)
	waitUI(t, w)
	if a == nil || b == nil || a.id == b.id || a.savedID != query.ID || b.savedRevision != query.Revision || a.title != query.Title || a.scope.Text != "main" || a.schemaName != "public" || len(a.lastResults) != 0 {
		t.Fatal("named query lost context, reused a draft, or ran automatically")
	}
	a.editor.SetText("SELECT 43;")
	got, err := w.Store.SavedQuery(t.Context(), query.ID)
	if err != nil || got.Text != query.Text || b.editor.Text != query.Text {
		t.Fatal("editing one tab mutated the saved document or another tab", err)
	}
	if a.draft().Schema != "public" || a.draft().Title != query.Title {
		t.Fatal("draft did not preserve title and schema")
	}
}

func TestSQLFileNamesCannotEscapeSelectedDirectory(t *testing.T) {
	for _, name := range []string{"", "..", "../query", "dir/query", `dir\query`, "query\x00"} {
		if sqlFileName(name) != "" {
			t.Fatal("unsafe filename accepted")
		}
	}
	if sqlFileName("报表") != "报表.sql" || sqlFileName("report.SQL") != "report.SQL" {
		t.Fatal("file extension changed unexpectedly")
	}
}
