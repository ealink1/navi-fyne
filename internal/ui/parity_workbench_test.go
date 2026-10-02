package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/ealink1/navi-fyne/internal/application"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/drivers"
	"github.com/ealink1/navi-fyne/internal/infra/release"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
	"github.com/ealink1/navi-fyne/internal/infra/state"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

type parityFixture struct{ uiFixtureClient }

func (parityFixture) TableInfo(context.Context, string, string) (domain.TableInfo, error) {
	return domain.TableInfo{Columns: []connection.ColumnDefinition{{Name: "id", Type: "INTEGER", Key: "PRI", Nullable: "NO"}, {Name: "name", Type: "TEXT", Nullable: "YES"}}, DDL: "CREATE TABLE items(id INTEGER PRIMARY KEY,name TEXT)"}, nil
}
func (parityFixture) Objects(context.Context, string) ([]domain.Object, error) {
	return []domain.Object{{Name: "items", Schema: "public", Kind: "table"}, {Name: "active_items", Schema: "public", Kind: "view"}}, nil
}
func (parityFixture) Execute(_ context.Context, request domain.Execution) ([]domain.Result, error) {
	if strings.Contains(request.Text, "COUNT(*)") {
		return []domain.Result{{Rows: [][]any{{int64(120)}}}}, nil
	}
	if strings.Contains(request.Text, "Fyne parity") {
		return []domain.Result{{Columns: []domain.Column{{Name: "id"}, {Name: "name"}, {Name: "optional_value"}}, Rows: [][]any{{int64(1), "Fyne parity", nil}}}}, nil
	}
	return []domain.Result{{Columns: []domain.Column{{Name: "id"}, {Name: "name"}}, Rows: [][]any{{int64(1), "first"}, {int64(2), nil}}}}, nil
}
func parityWindow(t *testing.T) (*Window, domain.Profile) {
	t.Helper()
	app := test.NewApp()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	profiles := &application.Profiles{Store: store, Vault: secrets.New()}
	p, err := profiles.Save(context.Background(), domain.Profile{Name: "演示连接", Environment: "test", Config: connection.ConnectionConfig{Type: "postgres", Host: "127.0.0.1", Database: "main"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := application.NewEngine(profiles)
	engine.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return parityFixture{}, nil }
	w := newTestWindow(t, app, Dependencies{Profiles: profiles, Engine: engine, Drivers: &drivers.Manager{Root: filepath.Join(root, "drivers")}, Releases: release.New(""), Root: root, Close: func() error { return nil }})
	w.Show()
	waitReady(t, w)
	waitUI(t, w)
	t.Cleanup(func() {
		w.jobs.stop()
		engine.Close()
		store.Close()
		w.Window.SetCloseIntercept(nil)
		w.Window.Close()
		app.Quit()
	})
	return w, p
}
func TestNativeTreeSchemaAndObjectFiltersOpenDataTab(t *testing.T) {
	w, p := parityWindow(t)
	n := w.sidebar
	if !n.tree.IsBranch("") || len(n.tree.ChildUIDs("")) != 1 {
		t.Fatal("root prevents connection rows from being rendered")
	}
	root := n.roots[0]
	n.tree.OpenBranch(root)
	waitUI(t, w)
	database := n.nodes[root].children[0]
	n.tree.OpenBranch(database)
	waitUI(t, w)
	schema := n.nodes[n.nodes[database].children[0]]
	if schema.kind != "schema" || schema.label != "public" {
		t.Fatal("missing schema level", schema)
	}
	n.kindFilter = "view"
	filtered := n.filteredChildren(schema)
	if len(filtered) != 1 || !strings.HasSuffix(filtered[0], "/view") {
		t.Fatal("view filter shows other categories", filtered)
	}
	n.kindFilter = ""
	tableNode := n.nodes[n.nodes[schema.children[0]].children[0]]
	n.activate(tableNode.id)
	waitUI(t, w)
	workbench := w.tables[w.tabs.Selected()]
	if workbench == nil || workbench.profile.ID != p.ID || workbench.page.Total != 120 || workbench.page.Result.Rows[1][1] != nil {
		t.Fatal("table did not open with actual page and null metadata")
	}
	n.activate(tableNode.id)
	waitUI(t, w)
	if len(w.tables) != 1 {
		t.Fatal("table opened duplicate tabs")
	}
	w.closeTab(workbench.item)
	if !workbench.closed || len(w.tables) != 0 {
		t.Fatal("closed table still receives results")
	}
}
func TestTableEditsDoNotMutateLoadedRowsOrDiscardOnPaging(t *testing.T) {
	w, p := parityWindow(t)
	table := w.openTable(p, domain.Object{Name: "items", Schema: "public", Scope: "main", Kind: "table"})
	waitUI(t, w)
	captureParity(t, w, "parity-table.png")
	if err := table.editCell(0, 1, "changed", false); err != nil {
		t.Fatal(err)
	}
	if table.page.Result.Rows[0][1] != "first" || table.cellValue(0, 1) != "changed" || !table.dirty() {
		t.Fatal("editing changed the original locator")
	}
	table.gotoPage(2)
	if table.request.Page != 1 {
		t.Fatal("page change discarded pending edits")
	}
	table.selected[1] = true
	table.deleteRows()
	changes := table.changes()
	if len(changes.Rows) != 2 || changes.Rows[1].Kind != "delete" || changes.Rows[1].Original["name"] != nil {
		t.Fatal(changes)
	}
	table.clearEdits()
	table.addRow()
	if len(table.inserts) != 1 {
		t.Fatal("add row did not create draft")
	}
	if err := table.editCell(2, 0, "3", false); err != nil {
		t.Fatal(err)
	}
	if err := table.editCell(2, 1, "inserted", false); err != nil {
		t.Fatal(err)
	}
	if table.changes().Rows[0].Values["id"] != int64(3) {
		t.Fatal("integer editing changed value type")
	}
	table.clearEdits()
	table.gotoPage(2)
	waitUI(t, w)
	if table.page.Page != 2 {
		t.Fatal("valid pagination did not complete")
	}
}
func TestNativeCodeEditorPreservesTypingUndoAndColoredSource(t *testing.T) {
	w, p := parityWindow(t)
	s := w.openWorkspace(p, domain.Draft{ID: "colored", ProfileID: p.ID, Text: "SELECT 1;\n-- 中文", Scope: "main"})
	waitUI(t, w)
	if s.code == nil {
		t.Fatal("SQL editor missing")
	}
	s.editor.CursorRow = 0
	s.editor.CursorColumn = 7
	s.editor.TypedRune('2')
	if s.editor.Text != "SELECT 21;\n-- 中文" {
		t.Fatal(s.editor.Text)
	}
	s.editor.Undo()
	if s.editor.Text != "SELECT 1;\n-- 中文" {
		t.Fatal("undo lost source text", s.editor.Text)
	}
	s.editor.SetText("SELECT 1 AS id, 'Fyne parity' AS name, NULL AS optional_value;")
	s.refreshObjects()
	waitUI(t, w)
	s.run(false, "")
	waitUI(t, w)
	captureParity(t, w, "parity-query.png")
	w.App.Settings().SetTheme(Theme{Dark: true})
	captureParity(t, w, "parity-query-dark.png")
	if source := string(icon("search").Content()); !strings.Contains(source, `fill="none"`) || !strings.Contains(source, `stroke="#`) {
		t.Fatal("outline icon was turned into a filled silhouette", source)
	}
}

// This is a software renderer capture, not native desktop or IME validation.
func captureParity(t *testing.T, w *Window, name string) {
	t.Helper()
	directory := os.Getenv("NAVIFYNE_UI_CAPTURE_DIR")
	if directory == "" {
		return
	}
	w.Window.Resize(fyne.NewSize(1470, 923))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, w.Window.Canvas().Capture())
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("capture failed", err, closeErr)
	}
}
