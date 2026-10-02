package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

func (w *Window) buildDocuments() {
	w.docButtons = make(map[*container.TabItem]*documentTab)
	w.docStrip = container.NewHBox()
	strip := container.NewHScroll(w.docStrip)
	strip.SetMinSize(fyne.NewSize(0, 48))
	controls := container.NewHBox(action("", "add-row", w.newSelectedQuery), action("", "connection-menu", w.documentMenu))
	w.docHeader = container.NewStack(container.NewBorder(nil, nil, nil, controls, strip))
	w.docBody = container.NewStack()
	w.docHost = container.NewBorder(w.docHeader, nil, nil, nil, w.docBody)
}

// Keep header widgets stable while switching, renaming or refreshing pages.
// Recreating their renderers keeps obsolete callback closures alive in Fyne's
// renderer cache; detach those callbacks as soon as a page leaves the strip.
func (w *Window) syncDocuments() {
	if w.docStrip == nil {
		return
	}
	selected := w.tabs.Selected()
	buttons := make([]fyne.CanvasObject, 0, len(w.tabs.Items))
	active := make(map[*container.TabItem]bool, len(w.tabs.Items))
	for _, item := range w.tabs.Items {
		active[item] = true
		title, subtitle := w.documentLabel(item)
		button := w.docButtons[item]
		if button == nil {
			button = newDocumentTab(title, subtitle, item == selected, func() { w.tabs.Select(item) }, func() { w.closeTab(item) })
			w.docButtons[item] = button
		}
		button.title, button.subtitle, button.selected = title, subtitle, item == selected
		button.Refresh()
		buttons = append(buttons, button)
	}
	for item, button := range w.docButtons {
		if !active[item] {
			button.selectTab, button.closeTab = nil, nil
			delete(w.docButtons, item)
		}
	}
	w.docStrip.Objects = buttons
	w.docStrip.Refresh()
	if selected != nil {
		w.docBody.Objects = []fyne.CanvasObject{selected.Content}
	} else {
		w.docBody.Objects = nil
	}
	w.docBody.Refresh()
}

func (w *Window) documentLabel(item *container.TabItem) (title, subtitle string) {
	title = item.Text
	if space := w.workspaces[item]; space != nil {
		title = "新建查询"
		if space.title != "" && space.title != space.profile.Name {
			title = space.title
		}
		subtitle = "SQL · [" + space.profile.Name + " · " + space.scope.Text + "]"
	}
	if table := w.tables[item]; table != nil {
		title = table.object.Name
		subtitle = "TABLE · [" + table.profile.Name + " · " + table.object.Scope + "]"
	}
	return title, subtitle
}
