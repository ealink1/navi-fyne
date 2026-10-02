package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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

func interactionWindow(t *testing.T, factory func(context.Context, domain.Profile) (adapter.Client, error)) *Window {
	t.Helper()
	app := test.NewApp()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	profiles := &application.Profiles{Store: store, Vault: secrets.New()}
	for _, name := range []string{"数据库 A", "数据库 B"} {
		if _, err = profiles.Save(context.Background(), domain.Profile{Name: name, ReadOnly: true, Config: connection.ConnectionConfig{Type: "mysql", Host: "127.0.0.1"}}); err != nil {
			t.Fatal(err)
		}
	}
	engine := application.NewEngine(profiles)
	engine.Factory = factory
	w := newTestWindow(t, app, Dependencies{Profiles: profiles, Engine: engine, Drivers: &drivers.Manager{Root: filepath.Join(root, "drivers")}, Releases: release.New(""), Root: root, Version: "0.1.0", Close: func() error { return nil }})
	t.Cleanup(func() {
		w.jobs.stop()
		w.Window.SetCloseIntercept(nil)
		w.Window.Close()
		_ = engine.Close()
		_ = store.Close()
		app.Quit()
	})
	w.Show()
	waitReady(t, w)
	waitUI(t, w)
	return w
}

func TestFilteredConnectionCannotOpenPreviousSelection(t *testing.T) {
	w := interactionWindow(t, func(context.Context, domain.Profile) (adapter.Client, error) { return uiFixtureClient{}, nil })
	a, b := w.profiles[0], w.profiles[1]
	w.list.Select(0)
	w.search.SetText(b.Name)
	if w.selected != "" || len(w.visible) != 1 || w.visible[0].ID != b.ID {
		t.Fatal("filtered row still refers to a hidden connection")
	}
	test.Tap(findButton(w.Window.Content(), "新建查询"))
	if len(w.workspaces) != 0 || w.status.Text != "请先选择一个连接" {
		t.Fatal("button opened an unselected hidden connection")
	}
	row := w.list.CreateItem().(*connectionRow)
	w.list.UpdateItem(0, row)
	test.Tap(row)
	if w.selected != b.ID || len(w.workspaces) != 0 {
		t.Fatal("single tap must select the visible row")
	}
	w.search.SetText("")
	if w.selected != b.ID || w.visible[0].ID != a.ID {
		t.Fatal("clearing the filter changed connection identity")
	}
	// A pooled row must be rebound to the visible identity after a filter change.
	w.list.UpdateItem(1, row)
	test.DoubleTap(row)
	waitUI(t, w)
	space := w.workspaces[w.tabs.Selected()]
	if space == nil || space.profile.ID != b.ID || space.scope.Text != "main" || len(space.objects) != 1 {
		t.Fatal("double tap did not open and load the filtered connection")
	}
	items := len(w.workspaces)
	test.DoubleTap(row)
	if len(w.workspaces) != items {
		t.Fatal("opening the same connection duplicated its workspace")
	}
	w.tabs.CreateTab()
	waitUI(t, w)
	if len(w.workspaces) != items+1 {
		t.Fatal("the plus button no longer creates a separate query")
	}
}

func TestConnectionFailureStillOpensWorkbenchAndExplainsFailure(t *testing.T) {
	w := interactionWindow(t, func(context.Context, domain.Profile) (adapter.Client, error) {
		return nil, errors.New("fixture connection rejected")
	})
	w.list.Select(0)
	test.Tap(findButton(w.Window.Content(), "新建查询"))
	waitUI(t, w)
	space := w.workspaces[w.tabs.Selected()]
	if space == nil || space.busy || !space.stop.Disabled() || !strings.Contains(space.status.Text, "fixture connection rejected") {
		t.Fatal("connection failure was hidden behind the welcome page")
	}
}
