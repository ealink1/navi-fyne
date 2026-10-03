package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (s *workspaceSwitcher) utilityBar() fyne.CanvasObject {
	s.themeButton = widget.NewButton("日间", s.owner.toggleAppearance)
	hint := action("", "insight", s.owner.aiEntry)
	settings := action("", "settings", s.openSettings)
	return container.NewHBox(
		container.NewGridWrap(fyne.NewSize(72, 38), s.themeButton),
		container.NewGridWrap(fyne.NewSize(38, 38), hint),
		container.NewGridWrap(fyne.NewSize(38, 38), settings),
	)
}

// Settings live in SQL's document host; show that host without replacing or
// disconnecting the cached Shell and Note workspaces.
func (s *workspaceSwitcher) openSettings() {
	if s.owner.shuttingDown {
		return
	}
	s.selectMode(0)
	s.owner.settingsPage()
}

func (s *workspaceSwitcher) utilityAction(action int) {
	if s.owner.shuttingDown {
		return
	}
	switch action {
	case 0:
		s.owner.toggleAppearance()
	case 1:
		s.owner.aiEntry()
	case 2:
		s.openSettings()
	}
}
