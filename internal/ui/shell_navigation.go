package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type shellNavigationTheme struct{ shellTheme }

func (t shellNavigationTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameInlineIcon {
		return 22
	}
	return t.shellTheme.Size(name)
}

func (s *shellWorkspace) buildNavigation() fyne.CanvasObject {
	s.navigation = []*shellNavButton{
		newShellNav(s, "server", "主机", s.showHosts),
		newShellNav(s, "terminal", "终端", s.showTerminals),
		newShellNav(s, "settings", "设置", s.showSettings),
	}
	items := []fyne.CanvasObject{shellFixed(shellButton("", "chevron-right", false, func() { s.toggleNavigation() }), 40, 40), shellFixed(layout.NewSpacer(), 0, 16)}
	for _, button := range s.navigation[:2] {
		items = append(items, shellFixed(button, 40, 40))
	}
	s.navHint = widget.NewLabel("")
	s.navHint.Alignment = fyne.TextAlignCenter
	bottom := shellVBox(shellFixed(s.navigation[2], 40, 40), shellFixed(s.navHint, 0, 24))
	rail := shellBorder(shellVBox(items...), bottom, nil, nil, layout.NewSpacer())
	s.rail = shellFixed(container.NewStack(shellRectangle(shellRailColor, 0, nil), shellInset(container.NewThemeOverride(rail, shellNavigationTheme{shellTheme: newShellTheme()}), 12)), 64, 0)
	return s.rail
}

func (s *shellWorkspace) showNavHint(text string) {
	if s.navHint != nil {
		s.navHint.SetText(text)
	}
}

func (s *shellWorkspace) toggleNavigation() {
	s.expanded = !s.expanded
	for _, button := range s.navigation {
		button.SetText("")
		if s.expanded {
			button.SetText(button.tooltip)
		}
	}
	if frame, ok := s.rail.Layout.(*shellFixedLayout); ok {
		frame.width = 64
		if s.expanded {
			frame.width = 144
		}
	}
	s.content.Refresh()
}
