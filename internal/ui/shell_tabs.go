package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
	selectTab.Text = fitShellTabTitle(item.Text, 100)
	closeTab := shellButton("", "x", false, func() {
		s.tabs.CloseIntercept(item)
		s.showTerminals()
	})
	closeTab.SetIcon(shellIcon("x", false))
	content := shellBorder(nil, nil, nil, shellFixed(closeTab, 24, 26), shellButtonView(selectTab))
	fill := shellRailColor
	if s.tabs.Selected() == item {
		fill = shellColor(theme.ColorNameSelection)
	}
	panel := shellPanel(content, fill, 5, 0)
	if s.tabs.Selected() == item {
		panel.Objects[0].(*shellPrimitive).stroke = shellAccentColor
	}
	return shellHBox(shellFixed(panel, 160, 28), shellFixed(layout.NewSpacer(), 5, 0))
}

func truncateShellTitle(text string, limit int) string {
	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return text
}

func fitShellTabTitle(text string, width float32) string {
	text = truncateShellTitle(text, 64)
	if fyne.MeasureText(text, 12, fyne.TextStyle{}).Width <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		candidate := string(runes) + "…"
		if fyne.MeasureText(candidate, 12, fyne.TextStyle{}).Width <= width {
			return candidate
		}
	}
	return "…"
}
