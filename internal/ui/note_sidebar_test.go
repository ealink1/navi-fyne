package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	fynetest "fyne.io/fyne/v2/test"
)

func TestNoteSidebarGroupsSearchCollapseAndNewNote(t *testing.T) {
	_, n := noteTestWindow(t)
	n.newNote()
	defaultID := n.selected
	n.title.SetText("未分组笔记")
	for _, name := range []string{"工作记录", "学习笔记", "空分组"} {
		if err := n.addGroup(name); err != nil {
			t.Fatal(err)
		}
	}
	workID, studyID := n.book.Groups[0].ID, n.book.Groups[1].ID
	workKey, studyKey := noteGroupNode(workID), noteGroupNode(studyID)
	n.list.Select(workKey)
	n.newNote()
	workNoteID := n.selected
	n.title.SetText("部署日志")
	n.editor.SetText("服务 alpha")
	n.filter()
	if n.current().GroupID != workID || !slices.Contains(n.groupNotes[workKey], noteLeafNode(workNoteID)) || !slices.Contains(n.groupNotes[noteGroupNode("")], noteLeafNode(defaultID)) {
		t.Fatal("note hierarchy or new-note target group incorrect")
	}
	if len(n.groupNodes) != 4 {
		t.Fatal("default or empty group missing")
	}
	n.list.Resize(fyne.NewSize(280, 600))
	markup := fynetest.RenderObjectToMarkup(n.list)
	for _, text := range []string{"工作记录", "部署日志", "默认分组", "空分组"} {
		if !strings.Contains(markup, text) {
			t.Fatalf("sidebar did not render %q", text)
		}
	}
	n.list.CloseBranch(workKey)
	n.filter()
	if n.list.IsBranchOpen(workKey) {
		t.Fatal("refresh reopened a manually collapsed group")
	}
	n.search.SetText("alpha")
	if len(n.groupNodes) != 1 || n.groupNodes[0] != workKey || len(n.visible) != 1 || !n.list.IsBranchOpen(workKey) {
		t.Fatal("search did not reveal matching note inside its group")
	}
	n.search.SetText("")
	if n.list.IsBranchOpen(workKey) {
		t.Fatal("search cleared the user's collapsed state")
	}
	n.search.SetText("工作记录")
	if len(n.visible) != 1 || n.visible[0] != workNoteID {
		t.Fatal("group-name search omitted its notes")
	}
	n.search.SetText("")
	n.selectNote(workNoteID)
	n.groupPicker.SetSelected("学习笔记")
	if len(n.groupNotes[workKey]) != 0 || len(n.groupNotes[studyKey]) != 1 || n.activeGroup != studyID || !n.list.IsBranchOpen(studyKey) {
		t.Fatal("moving a note did not update/reveal its new group")
	}
	n.toggleDeleted()
	if len(n.groupNotes[studyKey]) != 0 {
		t.Fatal("trashed note stayed in normal hierarchy")
	}
	n.trash = true
	n.filter()
	if len(n.groupNodes) != 1 || n.groupNodes[0] != studyKey || len(n.visible) != 1 {
		t.Fatal("trash lost the note's group hierarchy")
	}
	n.selectNote(workNoteID)
	n.toggleDeleted()
	n.trash = false
	n.filter()
	if len(n.groupNotes[studyKey]) != 1 {
		t.Fatal("restore lost the group membership")
	}
	n.removeGroup("学习笔记")
	if len(n.groupNotes[noteGroupNode("")]) != 2 || slices.Contains(n.groupNodes, studyKey) || n.activeGroup != "" {
		t.Fatal("removed group did not move notes back to default")
	}
}

func TestNoteSidebarSoftwareFixtures(t *testing.T) {
	if os.Getenv("NAVIFYNE_UI_CAPTURE_DIR") == "" {
		t.Skip("optional software render capture")
	}
	w, n := noteTestWindow(t)
	n.newNote()
	n.title.SetText("快速记录")
	n.editor.SetText("整理今天的工作事项")
	for _, name := range []string{"工作记录", "学习笔记"} {
		if err := n.addGroup(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"服务器维护", "发布计划"} {
		n.list.Select(noteGroupNode(n.book.Groups[0].ID))
		n.newNote()
		n.title.SetText(name)
		n.editor.SetText("记录部署过程与验证结果")
	}
	n.filter()
	for _, dark := range []bool{false, true} {
		w.setDark(dark)
		mode := map[bool]string{false: "light", true: "dark"}[dark]
		captureShellFixture(t, w, "note-sidebar-"+mode+".png")
		captureShellFixtureSize(t, w, "note-sidebar-small-"+mode+".png", fyne.NewSize(960, 640))
	}
}
