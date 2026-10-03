package ui

import (
	"testing"

	"fyne.io/fyne/v2/widget"
	transport "github.com/ealink1/super-link/internal/infra/shell"
)

func TestShellFileSearchKeepsSelectionMappedToRemoteEntry(t *testing.T) {
	w := shellTestWindow(t)
	w.switcher.selectMode(1)
	waitUI(t, w)
	p := w.switcher.shell.newPane("fixture")
	f := &shellFiles{pane: p, selected: -1, directory: widget.NewEntry(), status: widget.NewLabel("")}
	f.content = newShellFilesView(f)
	f.files = []transport.File{{Name: "README.md"}, {Name: "docs", Directory: true}, {Name: "DOCS.txt"}}
	f.search.SetText("docs")
	if len(f.shown) != 2 {
		t.Fatal("search lost case-insensitive matches")
	}
	f.list.Select(0)
	if f.selected != 1 || !f.files[f.selected].Directory {
		t.Fatal("filtered selection points at wrong remote file")
	}
	f.search.SetText("README")
	if f.selected != -1 || len(f.shown) != 1 || f.shown[0] != 0 {
		t.Fatal("filter retained stale selection")
	}
	f.search.SetText("missing")
	if len(f.shown) != 0 || f.selected != -1 {
		t.Fatal("empty search retained a downloadable file")
	}
	p.stop()
	pumpShell(t, w, func() bool { return w.jobs.workers.Load() == 0 })
}
