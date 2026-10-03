package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/application"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
)

func noteTestWindow(t *testing.T) (*Window, *noteWorkspace) {
	t.Helper()
	w := shellTestWindow(t)
	w.Profiles.Vault = secrets.NewLocal(w.Root)
	if w.switcher.note != nil {
		t.Fatal("Note eagerly allocated")
	}
	w.switcher.selectMode(2)
	n := w.switcher.note
	pumpShell(t, w, func() bool { return !n.loading })
	if !n.loaded {
		t.Fatal(n.status.Text)
	}
	return w, n
}
func TestNoteEditingGroupingTrashAndWorkspaceIsolation(t *testing.T) {
	w, n := noteTestWindow(t)
	sql := w.switcher.sql
	n.newNote()
	id := n.selected
	n.title.SetText("部署记录")
	n.editor.SetText("# 日志\n\n服务 alpha")
	n.tags.SetText("运维")
	if err := n.addGroup("生产文档"); err != nil {
		t.Fatal(err)
	}
	n.groupPicker.SetSelected("生产文档")
	if n.current().GroupID == "" {
		t.Fatal("group not assigned")
	}
	if err := n.renameGroup("生产文档", "服务文档"); err != nil {
		t.Fatal(err)
	}
	if n.groupPicker.Selected != "服务文档" {
		t.Fatal("renamed group picker stale")
	}
	n.search.SetText("alpha")
	if len(n.visible) != 1 {
		t.Fatal("body search failed")
	}
	n.search.SetText("missing")
	if len(n.visible) != 0 {
		t.Fatal("search retained row")
	}
	n.search.SetText("")
	n.viewPicker.SetSelected("分屏")
	if _, ok := n.editorHost.Objects[0].(*container.Split); !ok {
		t.Fatal("split replaced fixed region")
	}
	n.viewPicker.SetSelected("预览")
	if len(n.preview.Segments) == 0 {
		t.Fatal("preview empty")
	}
	w.switcher.selectMode(0)
	if w.switcher.body.Objects[0] != sql {
		t.Fatal("SQL replaced")
	}
	w.switcher.selectMode(2)
	if w.switcher.note != n || n.selected != id || n.editor.Text != "# 日志\n\n服务 alpha" {
		t.Fatal("workspace switch lost note")
	}
	n.toggleDeleted()
	if len(n.visible) != 0 {
		t.Fatal("deleted note visible")
	}
	n.trash = true
	n.filter()
	if len(n.visible) != 1 {
		t.Fatal("trash lost note")
	}
	n.selectNote(id)
	if !n.editor.Disabled() {
		t.Fatal("trash note editable")
	}
	n.toggleDeleted()
	n.trash = false
	n.filter()
	n.selectNote(id)
	n.removeGroup("服务文档")
	if n.current().GroupID != "" || n.editor.Disabled() {
		t.Fatal("group removal lost note")
	}
	n.save()
	pumpShell(t, w, func() bool { return !n.saving })
	if n.savedRevision != n.editRevision {
		t.Fatal(n.status.Text)
	}
	restarted := &application.Notebook{Store: w.Store, Vault: secrets.NewLocal(w.Root)}
	loaded, err := restarted.Load(context.Background())
	if err != nil || len(loaded.Notes) != 1 || loaded.Notes[0].Deleted {
		t.Fatal("saved edit failed", err)
	}
}
func TestNoteImmediateShutdownFlushesLastEdit(t *testing.T) {
	w, n := noteTestWindow(t)
	n.newNote()
	n.editor.SetText("last edit before debounce")
	flush := w.noteShutdownSnapshot()
	w.jobs.stop()
	if err := flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded, err := n.service.Load(context.Background())
	if err != nil || loaded.Notes[0].Body != "last edit before debounce" {
		t.Fatal("last edit lost", err)
	}
}
func TestNoteSaveFailureRetainsEditingAndCanRetry(t *testing.T) {
	w, n := noteTestWindow(t)
	n.newNote()
	n.editor.SetText("retry fixture")
	n.service.Vault = secrets.New()
	n.save()
	pumpShell(t, w, func() bool { return !n.saving })
	if n.editor.Text != "retry fixture" || n.savedRevision == n.editRevision || !strings.Contains(n.status.Text, "失败") {
		t.Fatal("failed save discarded dirty edit")
	}
	n.service.Vault = secrets.NewLocal(w.Root)
	n.save()
	pumpShell(t, w, func() bool { return !n.saving })
	if n.savedRevision != n.editRevision {
		t.Fatal(n.status.Text)
	}
}
func TestNoteSafeMarkdownAndFormatting(t *testing.T) {
	w, n := noteTestWindow(t)
	n.newNote()
	n.editor.SetText("![remote](https://example.com/image.png)\n\n[local](file:///tmp/private)\n\n[web](https://example.com)")
	n.viewPicker.SetSelected("预览")
	var inspect func([]widget.RichTextSegment)
	inspect = func(segments []widget.RichTextSegment) {
		for _, s := range segments {
			switch v := s.(type) {
			case *widget.ImageSegment:
				t.Fatal("image fetch possible")
			case *widget.HyperlinkSegment:
				if v.URL.Scheme != "https" {
					t.Fatal("unsafe link")
				}
			case *widget.ParagraphSegment:
				inspect(v.Texts)
			}
		}
	}
	inspect(n.preview.Segments)
	n.viewPicker.SetSelected("编辑")
	n.editor.SetText("")
	n.insertFormat("**", "**")
	if n.editor.Text != "**内容**" {
		t.Fatal("format insertion failed")
	}
	n.editor.Undo()
	if n.editor.Text != "" {
		t.Fatal("undo failed")
	}
	if got := formatNoteSelection("a\nb", "# ", ""); got != "# a\n# b" {
		t.Fatal(got)
	}
	if got := noteFilename("../../bad:name"); strings.ContainsAny(got, "/:") || filepath.Ext(got) != ".md" {
		t.Fatal("unsafe export name")
	}
	_ = w
}
func TestNoteSharedAppearance(t *testing.T) {
	w, n := noteTestWindow(t)
	w.Window.Resize(fyne.NewSize(960, 640))
	n.newNote()
	n.editor.SetText("# fixture")
	for _, mode := range []string{"light", "dark"} {
		w.setDark(mode == "dark")
		n.content.Refresh()
		if n.content.Size().Width == 0 {
			t.Fatal("note layout empty")
		}
	}
}
