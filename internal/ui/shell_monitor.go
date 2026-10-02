package ui

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	transport "github.com/ealink1/navi-fyne/internal/infra/shell"
)

type shellMonitor struct {
	content *fyne.Container
	refresh *widget.Button
}

func (p *shellPane) showMonitor() {
	remote, ok := p.session.(*transport.Remote)
	if !ok || p.ended {
		p.status.SetText("服务器监控需要已连接的 Linux SSH 会话")
		return
	}
	if p.monitorPane == nil {
		p.monitorPane = newShellMonitor(p, remote)
	}
	p.aux.Objects = []fyne.CanvasObject{p.monitorPane.content}
	p.aux.Show()
	p.aux.Refresh()
	p.workspace.content.Refresh()
}

func newShellMonitor(p *shellPane, remote *transport.Remote) *shellMonitor {
	label := widget.NewLabel("点击刷新读取服务器指标。")
	label.Wrapping = fyne.TextWrapWord
	refresh := shellButton("刷新指标", "refresh-cw", false, nil)
	refresh.OnTapped = func() {
		if p.closed || p.ended {
			return
		}
		refresh.Disable()
		label.SetText("正在读取…")
		p.workspace.owner.jobs.run(func(context.Context) (any, error) { return remote.Monitor(p.ctx) }, func(value any, err error) {
			refresh.Enable()
			if p.closed {
				return
			}
			if err != nil {
				label.SetText(err.Error())
				return
			}
			m := value.(transport.Metrics)
			label.SetText(fmt.Sprintf("%s\n\n负载\n%s\n\n内存\n%.1f / %.1f MiB\n\n运行时长（秒）\n%s\n\n根目录磁盘\n%s", m.System, m.Load, float64(m.MemoryUsed)/(1<<20), float64(m.MemoryTotal)/(1<<20), m.Uptime, m.Disk))
		})
	}
	header := shellVBox(shellText("服务器监控", 14, true, shellTextColor), shellLine(), shellOutlined(refresh))
	monitor := &shellMonitor{refresh: refresh, content: container.NewGridWrap(fyne.NewSize(410, 560), shellInset(shellBorder(header, nil, nil, nil, container.NewVScroll(shellLabel(label, 13))), 12))}
	refresh.OnTapped()
	return monitor
}
