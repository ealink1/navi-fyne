package ui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
)

func TestShellEditorRetainsCredentialsAndMetadata(t *testing.T) {
	w := shellTestWindow(t)
	w.switcher.selectMode(1)
	waitUI(t, w)
	s := w.switcher.shell
	vaultRoot := t.TempDir()
	s.service.Vault = secrets.NewLocal(vaultRoot)
	e := newShellHostEditor(s, domain.ShellHost{Port: 22, User: "tester", Remember: true})
	e.modal.popup.Show()
	e.name.SetText("开发服务器")
	e.address.SetText("127.0.0.1")
	e.group.SetText("开发环境")
	e.tags.SetText("开发, 测试")
	e.notes.SetText("用于界面自检")
	e.password.SetText("test-only-password")
	e.persist()
	pumpShell(t, w, func() bool { return len(s.hosts) == 1 })
	h, err := s.service.Get(context.Background(), s.hosts[0].ID)
	if err != nil || h.Password != "test-only-password" || h.Tags != "开发, 测试" || h.Notes != "用于界面自检" {
		t.Fatal("saved host editor state lost", err)
	}
	if e.password.Text != "" {
		t.Fatal("closed form retained password")
	}
	s.service.Vault = secrets.NewLocal(vaultRoot)
	restored, err := s.service.Get(context.Background(), h.ID)
	if err != nil || restored.Password != "test-only-password" {
		t.Fatal("remembered credentials did not survive vault reload", err)
	}
	s.search.SetText("服务器")
	s.group = "开发环境"
	s.tag = "测试"
	s.filterHosts()
	if len(s.visible) != 1 {
		t.Fatal("combined group/tag/search failed")
	}
	s.tag = "生产"
	s.filterHosts()
	if len(s.visible) != 0 {
		t.Fatal("tag filter ignored")
	}
	if _, err = w.Store.Profiles(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalFontSizeReflowsWithoutWindowResize(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	resized := 0
	surface := newTerminalSurface(func(int, int) { resized++ })
	defer surface.dispose()
	window := app.NewWindow("font reflow")
	window.SetContent(surface)
	window.Resize(fyne.NewSize(640, 400))
	window.Show()
	defer window.Close()
	columns, rows := surface.emulator.Width(), surface.emulator.Height()
	before := resized
	surface.setTextSize(20)
	if surface.emulator.Width() >= columns || surface.emulator.Height() >= rows || resized <= before {
		t.Fatal("font change did not update grid and PTY dimensions")
	}
	surface.setTextSize(12)
	if surface.emulator.Width() <= columns || surface.emulator.Height() <= rows {
		t.Fatal("smaller font did not expand grid")
	}
}

func TestShellGeometryAndThemeIsolation(t *testing.T) {
	w := shellTestWindow(t)
	sqlTheme := w.App.Settings().Theme()
	w.switcher.selectMode(1)
	waitUI(t, w)
	s := w.switcher.shell
	e := newShellHostEditor(s, domain.ShellHost{Port: 22, User: "root", Remember: true})
	e.modal.popup.Show()
	t.Logf("input min=%v actual=%v header=%v body=%v", e.name.MinSize(), e.name.Size(), e.modal.popup.MinSize(), e.modal.popup.Size())
	if e.name.Size().Height < e.name.MinSize().Height {
		t.Error("input is shorter than its renderer minimum")
	}
	e.modal.hide()
	s.toggleNavigation()
	if s.rail.MinSize().Width != 144 {
		t.Error("sidebar did not expand")
	}
	s.toggleNavigation()
	if s.rail.MinSize().Width != 64 {
		t.Error("sidebar did not collapse")
	}
	w.switcher.selectMode(0)
	if w.App.Settings().Theme() != sqlTheme {
		t.Fatal("Shell changed SQL theme")
	}
	if icon, ok := shellIcon("terminal", true).(*shellOutlineIcon); !ok || icon.shade != theme.ColorNamePrimary {
		t.Fatal("active icon does not follow the shared primary color")
	}
	for _, width := range []float32{620, 940, 1160} {
		s.hostList.Resize(fyne.NewSize(width, 600))
		if s.hostList.columns < 1 || s.hostList.columns > 3 {
			t.Fatal("card columns outside bounds")
		}
	}
}
