package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
)

// connectionRow owns both pointer gestures. Double tapping must also select
// the row: Fyne may dispatch a double tap without a preceding single tap.
type connectionRow struct {
	widget.BaseWidget
	label            *widget.Label
	onSelect, onOpen func()
}

func newConnectionRow() *connectionRow {
	row := &connectionRow{label: widget.NewLabel("")}
	row.ExtendBaseWidget(row)
	return row
}
func (r *connectionRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.label)
}
func (r *connectionRow) Tapped(*fyne.PointEvent) {
	if r.onSelect != nil {
		r.onSelect()
	}
}
func (r *connectionRow) DoubleTapped(event *fyne.PointEvent) {
	r.Tapped(event)
	if r.onOpen != nil {
		r.onOpen()
	}
}
func (w *Window) connectionList() *widget.List {
	list := widget.NewList(func() int { return len(w.visible) }, func() fyne.CanvasObject {
		return newConnectionRow()
	}, func(id widget.ListItemID, item fyne.CanvasObject) {
		p := w.visible[id]
		descriptor, _ := domain.Resolve(p.Config.Type)
		label := p.Name + "  ·  " + descriptor.Name
		if p.ReadOnly {
			label += "  [只读]"
		}
		row := item.(*connectionRow)
		row.label.SetText(label)
		row.onSelect = func() {
			w.list.Select(id)
			w.list.Highlight(id)
			w.Window.Canvas().Focus(w.list)
		}
		row.onOpen = w.openSelected
	})
	list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(w.visible) {
			w.selected = w.visible[id].ID
		}
	}
	return list
}
