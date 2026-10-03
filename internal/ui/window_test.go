package ui

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/application"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/drivers"
	"github.com/ealink1/super-link/internal/infra/release"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/infra/state"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

type uiFixtureClient struct{}

func (uiFixtureClient) Close() error                             { return nil }
func (uiFixtureClient) Scopes(context.Context) ([]string, error) { return []string{"main"}, nil }
func (uiFixtureClient) Objects(context.Context, string) ([]domain.Object, error) {
	return []domain.Object{{Name: "用户", Kind: "table"}}, nil
}
func (uiFixtureClient) Schema(context.Context, string, string) (string, error) {
	return "CREATE TABLE 用户 (id INTEGER)", nil
}
func (uiFixtureClient) Execute(context.Context, domain.Execution) ([]domain.Result, error) {
	return []domain.Result{{Columns: []domain.Column{{ID: "0", Name: "编号"}, {ID: "1", Name: "名称"}, {ID: "2", Name: "空值"}}, Rows: [][]any{{int64(9223372036854775807), "中文内容", nil}}}}, nil
}
func TestNativeWorkbenchReadOnlyResultsAndDraftLifecycle(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	profiles := &application.Profiles{Store: store, Vault: secrets.New()}
	p, err := profiles.Save(context.Background(), domain.Profile{Name: "本地 SQLite", ReadOnly: true, Config: connection.ConnectionConfig{Type: "sqlite", Host: ":memory:"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := application.NewEngine(profiles)
	engine.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return uiFixtureClient{}, nil }
	defer engine.Close()
	w := newTestWindow(t, app, Dependencies{Profiles: profiles, Engine: engine, Drivers: &drivers.Manager{Root: filepath.Join(root, "drivers")}, Releases: release.New(""), Root: root, Version: "0.1.0", Close: func() error { return nil }})
	w.Show()
	waitReady(t, w)
	waitUI(t, w)
	w.list.Select(0)
	open := findButton(w.Window.Content(), "新建查询")
	if open == nil {
		t.Fatal("workbench entry point missing")
	}
	test.Tap(open)
	waitUI(t, w)
	space := w.workspaces[w.tabs.Selected()]
	if space == nil || space.profile.ID != p.ID {
		t.Fatal("selected connection did not open through the workbench button")
	}
	if space.scope.Text != "main" || len(space.objects) != 1 {
		t.Fatal("opening did not load the available scope and objects")
	}
	// A double tap is its own gesture; a single-tap callback may not precede it.
	w.list.UnselectAll()
	w.selected = ""
	row := w.list.CreateItem().(*connectionRow)
	w.list.UpdateItem(0, row)
	test.DoubleTap(row)
	waitUI(t, w)
	if w.selected != p.ID || w.tabs.Selected() != space.item || len(w.workspaces) != 1 {
		t.Fatal("double tap failed to select and reuse the connection workbench")
	}
	if !space.write.Disabled() || space.stop.Disabled() == false {
		t.Fatal("incorrect initial write/stop controls")
	}
	space.run(false, "")
	waitUI(t, w)
	if len(space.lastResults) != 1 || space.busy || space.status.Text == "" {
		t.Fatal("native query completion missing")
	}
	if directory := os.Getenv("SUPERLINK_UI_CAPTURE_DIR"); directory != "" {
		assertRenderGeometry(t, w.Window.Content(), "")
		if err = os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(directory, "workbench.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, w.Window.Canvas().Capture()); err != nil {
			file.Close()
			t.Fatal(err)
		}
		file.Close()
	}
	w.closeTab(space.item)
	waitUI(t, w)
	if len(w.workspaces) != 0 {
		t.Fatal("closed tab remained open")
	}
	drafts, err := store.Drafts(context.Background())
	if err != nil || len(drafts) != 1 || !drafts[0].Closed || drafts[0].Text == "" {
		t.Fatal("closed draft lost or would reopen automatically")
	}
	w.openWorkspace(p, drafts[0])
	waitUI(t, w)
	space = w.workspaces[w.tabs.Selected()]
	space.editor.SetText("SELECT '最后输入尚未触发自动保存';")
	if err = w.FlushAfterRun(); err != nil {
		t.Fatal(err)
	}
	drafts, err = store.Drafts(context.Background())
	if err != nil || drafts[0].Closed || drafts[0].Text != space.editor.Text {
		t.Fatal("last edits lost during event-loop shutdown")
	}
	w.jobs.stop()
	w.Window.SetCloseIntercept(nil)
	w.Window.Close()
}

func assertRenderGeometry(t *testing.T, object fyne.CanvasObject, path string) {
	t.Helper()
	if !object.Visible() {
		return
	}
	path += fmt.Sprintf("/%T", object)
	if shape, ok := object.(*canvas.Rectangle); ok && (shape.Size().Width < 0 || shape.Size().Height < 0) {
		t.Fatalf("negative shape geometry %s: %v", path, shape.Size())
	}
	switch item := object.(type) {
	case *fyne.Container:
		for _, child := range item.Objects {
			assertRenderGeometry(t, child, path)
		}
	case fyne.Widget:
		for _, child := range test.WidgetRenderer(item).Objects() {
			assertRenderGeometry(t, child, path)
		}
	}
}

func findButton(object fyne.CanvasObject, text string) *widget.Button {
	if button, ok := object.(*widget.Button); ok && button.Text == text {
		return button
	}
	var children []fyne.CanvasObject
	switch object := object.(type) {
	case *fyne.Container:
		children = object.Objects
	case *container.Split:
		children = []fyne.CanvasObject{object.Leading, object.Trailing}
	case *container.ThemeOverride:
		children = []fyne.CanvasObject{object.Content}
	}
	for _, child := range children {
		if button := findButton(child, text); button != nil {
			return button
		}
	}
	return nil
}
