package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func newShellTab(s *shellWorkspace, item *container.TabItem) fyne.CanvasObject {
	selectTab := shellButton(item.Text, "terminal", false, func() {
		s.tabs.Select(item)
		s.showTerminals()
		if pane := s.panes[item]; pane != nil && !pane.ended {
			s.owner.Window.Canvas().Focus(pane.terminal)
		}
	})
	selectTab.Alignment = widget.ButtonAlignLeading
	selectTab.Text = truncateShellTitle(item.Text, 18)
	closeTab := shellButton("", "x", false, func() {
		s.tabs.CloseIntercept(item)
		s.showTerminals()
	})
	closeTab.SetIcon(shellIcon("x", false))
	content := shellBorder(nil, nil, nil, shellFixed(closeTab, 24, 28), selectTab)
	fill := shellBackground
	if s.tabs.Selected() == item {
		fill = shellRailColor
	}
	return shellFixed(shellPanel(content, fill, 4, 0), 150, 28)
}

func truncateShellTitle(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return text
}
