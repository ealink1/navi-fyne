package ui

import (
	"os"
	"testing"

	"fyne.io/fyne/v2"
)

// Software captures complement native AppKit observations and make small-window
// clipping visible without touching a daily workspace or opening a browser.
func TestNoteSoftwareRenderFixtures(t *testing.T) {
	if os.Getenv("SUPERLINK_UI_CAPTURE_DIR") == "" {
		t.Skip("optional software render capture")
	}
	w, n := noteTestWindow(t)
	w.Window.SetContent(n.content)
	n.newNote()
	n.title.SetText("服务器维护笔记")
	n.editor.SetText("# Deployment log\n\n- 检查服务状态\n- 记录部署版本\n\n```sh\nsystemctl status nginx\n```")
	for _, dark := range []bool{false, true} {
		w.setDark(dark)
		mode := map[bool]string{false: "light", true: "dark"}[dark]
		for _, view := range []string{"编辑", "预览", "分屏"} {
			n.viewPicker.SetSelected(view)
			captureShellFixture(t, w, "note-"+view+"-"+mode+".png")
			captureShellFixtureSize(t, w, "note-small-"+view+"-"+mode+".png", fyne.NewSize(960, 640))
		}
	}
}
