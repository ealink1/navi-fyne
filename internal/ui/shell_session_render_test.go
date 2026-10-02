package ui

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
	transport "github.com/ealink1/navi-fyne/internal/infra/shell"
)

func TestShellSSHSessionSoftwareFixtures(t *testing.T) {
	if os.Getenv("NAVIFYNE_UI_CAPTURE_DIR") == "" {
		t.Skip("optional software render capture")
	}
	w := shellTestWindow(t)
	w.switcher.selectMode(1)
	waitUI(t, w)
	s := w.switcher.shell
	w.Window.SetContent(s.content)
	p := s.newPane("开发服务器")
	p.host = &domain.ShellHost{Name: "开发服务器", Host: "127.0.0.1", Port: 2222, User: "tester"}
	p.terminal.feed([]byte("\x1b[32mtester@dev\x1b[0m:~$ pwd\r\n/home/tester\r\n\x1b[32mtester@dev\x1b[0m:~$ ls\r\n\x1b[34mprojects\x1b[0m  README.md\r\n\x1b[32mtester@dev\x1b[0m:~$ "))
	p.status.SetText("软件渲染夹具")
	for _, dark := range []bool{false, true} {
		w.setDark(dark)
		mode := map[bool]string{false: "light", true: "dark"}[dark]
		captureShellFixture(t, w, "session-"+mode+".png")
		d := newShellConnectDialog(p)
		d.model.record(transport.ConnectEvent{Stage: transport.ConnectTCP, State: transport.ConnectCompleted})
		d.model.record(transport.ConnectEvent{Stage: transport.ConnectAuth, State: transport.ConnectStarted})
		d.model.record(transport.ConnectEvent{Stage: transport.ConnectAuth, State: transport.ConnectCompleted})
		d.model.record(transport.ConnectEvent{Stage: transport.ConnectChannel, State: transport.ConnectStarted})
		d.render()
		d.busy.Stop()
		captureShellFixture(t, w, "connect-"+mode+".png")
		d.close()
		m := newShellMonitorView(p)
		m.apply(transport.Metrics{System: "Linux", Load: "0.12 0.24 0.36", MemoryTotal: 2 << 30, MemoryUsed: 1 << 30, Uptime: "90000 80000", Disk: "/dev/vda 41943040 10485760 31457280 25% /"})
		p.showAuxiliary("monitor", "服务器监控", m.content)
		captureShellFixture(t, w, "monitor-"+mode+".png")
		captureShellFixtureSize(t, w, "monitor-small-"+mode+".png", fyne.NewSize(960, 640))
		f := &shellFiles{pane: p, selected: -1, directory: widget.NewEntry(), status: widget.NewLabel("3 项 · 软件渲染夹具")}
		f.directory.SetText("/home/tester")
		f.content = newShellFilesView(f)
		f.files = []transport.File{{Name: "projects", Directory: true}, {Name: "README.md", Size: 2048, Modified: time.Date(2026, 10, 2, 12, 30, 0, 0, time.UTC)}, {Name: "long-file-name-for-truncation-check.txt", Size: 102400}}
		f.filterFiles()
		p.showAuxiliary("files", "文件管理 · SFTP", f.content)
		captureShellFixture(t, w, "files-"+mode+".png")
		captureShellFixtureSize(t, w, "files-small-"+mode+".png", fyne.NewSize(960, 640))
		p.hideAuxiliary()
	}
	p.stop()
	pumpShell(t, w, func() bool { return w.jobs.workers.Load() == 0 })
}
