package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
)

type workspaceSwitcher struct {
	owner        *Window
	sql          fyne.CanvasObject
	shell        *shellWorkspace
	body, bar    *fyne.Container
	buttons      [2]*widget.Button
	themeButton  *widget.Button
	mode         int
	nativeSelect func(int, bool)
	nativeClose  func()
}

func (w *Window) buildShell() {
	s := &workspaceSwitcher{owner: w, sql: w.buildSQLWorkspace()}
	w.switcher = s
	s.body = container.NewStack(s.sql)
	s.buttons[0] = widget.NewButton("SQL", func() { s.selectMode(0) })
	s.buttons[1] = widget.NewButton("Shell", func() { s.selectMode(1) })
	s.buttons[0].Importance = widget.HighImportance
	s.themeButton = widget.NewButton("日间", w.toggleAppearance)
	menu := container.NewHBox(container.NewGridWrap(fyne.NewSize(72, 38), s.themeButton), container.NewGridWrap(fyne.NewSize(110, 38), s.buttons[0]), container.NewGridWrap(fyne.NewSize(110, 38), s.buttons[1]))
	s.bar = container.NewBorder(nil, nil, menu, nil, layout.NewSpacer())
	w.Window.SetContent(container.NewBorder(s.bar, nil, nil, nil, s.body))
}

func (s *workspaceSwitcher) selectMode(mode int) {
	if s.owner.shuttingDown || mode < 0 || mode > 1 || s.mode == mode {
		return
	}
	s.owner.docTooltip.hide()
	s.owner.Window.Canvas().Unfocus()
	s.mode = mode
	s.owner.Window.SetPadded(mode != 1)
	if mode == 0 {
		s.body.Objects = []fyne.CanvasObject{s.sql}
	} else {
		if s.shell == nil {
			s.shell = newShellWorkspace(s.owner)
		}
		s.body.Objects = []fyne.CanvasObject{s.shell.content}
	}
	for i, button := range s.buttons {
		button.Importance = widget.LowImportance
		if i == mode {
			button.Importance = widget.HighImportance
		}
		button.Refresh()
	}
	s.body.Refresh()
	if s.nativeSelect != nil {
		s.nativeSelect(mode, s.owner.dark)
	}
}

func (s *workspaceSwitcher) installNative() {
	if s.nativeClose != nil {
		return
	}
	selectMode, closeTitlebar, ok := installWorkspaceTitlebar(s.owner.Window, s.selectMode, s.owner.toggleAppearance)
	if ok {
		s.nativeSelect, s.nativeClose = selectMode, closeTitlebar
		s.nativeSelect(s.mode, s.owner.dark)
		s.bar.Hide()
	}
}

func (s *workspaceSwitcher) stop() {
	if s.shell != nil {
		s.shell.stop()
	}
}

func (w *Window) newWorkspaceConnection() {
	if w.switcher.mode == 1 {
		w.switcher.shell.editHost(domain.ShellHost{Port: 22, User: "root", Remember: true})
		return
	}
	w.editProfile(domain.Profile{})
}
