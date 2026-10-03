package ui

import "testing"

func TestWorkspaceUtilitiesPreserveNoteWhileOpeningSettings(t *testing.T) {
	w, note := noteTestWindow(t)
	note.newNote()
	note.title.SetText("未保存的设置切换测试")
	note.editor.SetText("打开设置后仍保留正文")
	id := note.selected
	previousTheme := w.dark
	w.switcher.utilityAction(0)
	if w.dark == previousTheme || w.switcher.mode != 2 || w.switcher.note != note {
		t.Fatal("appearance utility changed the active workspace")
	}
	w.switcher.utilityAction(2)
	if w.switcher.mode != 0 || w.tabs.Selected().Text != "设置中心" {
		t.Fatal("settings utility opened an invisible SQL document")
	}
	w.switcher.selectMode(2)
	if w.switcher.note != note || note.selected != id || note.editor.Text != "打开设置后仍保留正文" {
		t.Fatal("opening settings discarded the note editor")
	}
	w.shuttingDown = true
	w.switcher.utilityAction(0)
	w.switcher.utilityAction(2)
	if w.dark == previousTheme || w.switcher.mode != 2 {
		t.Fatal("utilities remained active during shutdown")
	}
	w.shuttingDown = false
}
