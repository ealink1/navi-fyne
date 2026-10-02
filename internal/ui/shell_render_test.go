package ui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/ealink1/navi-fyne/internal/domain"
)

// These captures use Fyne's software test driver. They do not validate AppKit
// titlebar geometry, native keyboard events or installed iShell Pro fidelity.
func TestShellSoftwareRenderFixtures(t *testing.T) {
	if os.Getenv("NAVIFYNE_UI_CAPTURE_DIR") == "" {
		t.Skip("optional software render capture")
	}
	w := shellTestWindow(t)
	w.switcher.selectMode(1)
	waitUI(t, w)
	s := w.switcher.shell
	w.Window.SetContent(s.content)
	captureShellFixture(t, w, "shell-empty.png")
	for i, name := range []string{"开发服务器", "预发布服务器", "文档服务器", "测试服务器", "本地 Linux", "服务节点", "备份服务器"} {
		_, err := s.service.Save(context.Background(), domain.ShellHost{Name: name, Group: "开发环境", Host: "127.0.0.1", Port: 22 + i, User: "tester", Tags: "开发"})
		if err != nil {
			t.Fatal(err)
		}
	}
	s.reloadHosts()
	waitUI(t, w)
	s.showHosts()
	captureShellFixture(t, w, "shell-hosts.png")
	editor := newShellHostEditor(s, domain.ShellHost{Port: 22, User: "root", Remember: true})
	editor.modal.popup.Show()
	captureShellFixture(t, w, "shell-new-host.png")
	editor.modal.hide()
	p := s.newPane("本地终端 · 屏幕夹具")
	p.status.SetText("软件渲染夹具 · 未连接真实服务器")
	p.terminal.feed([]byte("\x1b[32mtester@dev\x1b[0m ~ $ echo 'Hello / 你好'\r\nHello / 你好\r\n\x1b[33mANSI 彩色文本\x1b[0m\r\n$ "))
	captureShellFixture(t, w, "shell-terminal.png")
	s.showSettings()
	captureShellFixture(t, w, "shell-settings.png")
	s.showHosts()
	captureShellFixtureSize(t, w, "shell-hosts-small.png", fyne.NewSize(960, 640))
	p.stop()
	waitUI(t, w)
}

func captureShellFixture(t *testing.T, w *Window, name string) {
	captureShellFixtureSize(t, w, name, fyne.NewSize(1470, 826))
}
func captureShellFixtureSize(t *testing.T, w *Window, name string, size fyne.Size) {
	t.Helper()
	w.Window.Resize(size)
	dir := os.Getenv("NAVIFYNE_UI_CAPTURE_DIR")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, w.Window.Canvas().Capture())
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
}
