package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Fyne's SelectEntry only calls OnChanged when an option is clicked. Wire the
// picker action explicitly so choosing a database reloads its schema metadata.
func (s *workspace) setScopeOptions(scopes []string) {
	s.scopePicker.SetOptions(scopes)
	button, ok := s.scopePicker.ActionItem.(*widget.Button)
	if !ok {
		button = widget.NewButton("", nil)
		s.scopePicker.ActionItem = button
	}
	button.SetIcon(theme.MenuDropDownIcon())
	button.OnTapped = func() {
		items := make([]*fyne.MenuItem, 0, len(scopes)+1)
		for _, scope := range scopes {
			label := scope
			if label == "" {
				label = "默认范围"
			}
			items = append(items, fyne.NewMenuItem(label, func() {
				if !s.closed && !s.busy {
					s.scope.SetText(scope)
					s.schemaName = ""
					s.refreshObjects()
				}
			}))
		}
		items = append(items, fyne.NewMenuItem("刷新数据库列表", s.loadScopes))
		position := s.owner.App.Driver().AbsolutePositionForObject(s.scopePicker).Add(fyne.NewPos(0, s.scopePicker.Size().Height))
		widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), s.owner.Window.Canvas(), position)
	}
	button.Importance = widget.LowImportance
	if s.scopePicker.Disabled() {
		button.Disable()
	}
	s.scopePicker.Refresh()
}
