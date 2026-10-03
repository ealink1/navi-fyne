package ui

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestNoteGroupsDialogSeparateInputsAndSelection(t *testing.T) {
	w, n := noteTestWindow(t)
	n.newNote()
	d := newNoteGroupsDialog(n)
	d.show()
	defer d.close()
	if w.Window.Canvas().Focused() != d.createName {
		t.Fatal("new-group input was not initially focused")
	}
	if !d.add.Disabled() || !d.rename.Disabled() || !d.remove.Disabled() || !d.editName.Disabled() {
		t.Fatal("empty group state exposes unavailable actions")
	}
	d.createName.SetText("工作记录")
	test.Tap(d.add)
	if len(n.book.Groups) != 1 || d.selected.Selected != "工作记录" || d.editName.Text != "工作记录" || !d.rename.Disabled() {
		t.Fatal("created group was not selected and filled for editing")
	}
	n.groupPicker.SetSelected("工作记录")
	d.selectGroup()
	if d.count.Text != "此分组有 1 篇笔记" {
		t.Fatal(d.count.Text)
	}
	d.createName.SetText("学习笔记")
	test.Tap(d.add)
	d.selected.SetSelected("工作记录")
	d.createName.SetText("新建草稿")
	d.editName.SetText("部署记录")
	test.Tap(d.rename)
	if d.selected.Selected != "部署记录" || d.createName.Text != "新建草稿" || n.current().GroupID != n.book.Groups[0].ID {
		t.Fatal("rename mixed inputs or changed note membership")
	}
	d.editName.SetText("学习笔记")
	test.Tap(d.rename)
	if d.selected.Selected != "部署记录" || d.editName.Text != "学习笔记" || d.feedback.Text == "分组名称已更新" {
		t.Fatal("invalid rename discarded input or reported success")
	}
	test.Tap(d.remove)
	if n.current().GroupID != "" || len(n.book.Notes) != 1 || d.selected.Selected != "学习笔记" {
		t.Fatal("remove deleted notes or left stale selection")
	}
	test.Tap(d.remove)
	if !d.selected.Disabled() || !d.rename.Disabled() || !d.remove.Disabled() || d.editName.Text != "" {
		t.Fatal("last group removal left stale actions")
	}
}

func TestNoteGroupsDialogSoftwareFixtures(t *testing.T) {
	if os.Getenv("NAVIFYNE_UI_CAPTURE_DIR") == "" {
		t.Skip("optional software render capture")
	}
	w, n := noteTestWindow(t)
	n.newNote()
	if err := n.addGroup("工作记录"); err != nil {
		t.Fatal(err)
	}
	n.groupPicker.SetSelected("工作记录")
	for _, dark := range []bool{false, true} {
		w.setDark(dark)
		d := newNoteGroupsDialog(n)
		d.show()
		mode := map[bool]string{false: "light", true: "dark"}[dark]
		captureShellFixture(t, w, "note-groups-"+mode+".png")
		d.close()
		w.Window.Resize(fyne.NewSize(960, 640))
		d = newNoteGroupsDialog(n)
		d.show()
		captureShellFixtureSize(t, w, "note-groups-small-"+mode+".png", fyne.NewSize(960, 640))
		d.close()
	}
}
