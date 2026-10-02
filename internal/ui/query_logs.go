package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
)

func (s *workspace) logPanel() fyne.CanvasObject {
	s.logView = widget.NewMultiLineEntry()
	s.logView.TextStyle = fyne.TextStyle{Monospace: true}
	s.logView.Disable()
	bar := container.NewHBox(widget.NewLabel("执行日志"), layout.NewSpacer(), action("清空", "trash", func() {
		s.logs = nil
		s.logView.SetText("")
		s.results.setCount(s.logTab, 0)
	}))
	return container.NewBorder(bar, nil, nil, nil, s.logView)
}

func (s *workspace) appendLog(started time.Time, err error, value any) {
	if s.logView == nil {
		return
	}
	state := "完成"
	if err != nil {
		state = "失败或中断"
	}
	rows := int64(0)
	if results, ok := value.([]domain.Result); ok {
		for _, r := range results {
			rows += int64(len(r.Rows)) + r.RowsAffected
		}
	}
	// Values and driver errors can contain secrets; only operation metadata is logged.
	line := fmt.Sprintf("[%s] %s · %s · %d 行 · %s", started.Format("15:04:05"), s.profile.Name, state, rows, time.Since(started).Round(time.Millisecond))
	s.logs = append(s.logs, line)
	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
	s.logView.SetText(strings.Join(s.logs, "\n"))
	s.results.setCount(s.logTab, len(s.logs))
}
