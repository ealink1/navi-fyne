package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type gridHeader struct {
	widget.BaseWidget
	model      gridModel
	table      *widget.Table
	name, kind *canvas.Text
	check      *widget.Check
	column     int
}

func newGridHeader(model gridModel, table *widget.Table) *gridHeader {
	h := &gridHeader{model: model, table: table, name: canvas.NewText("", theme.ForegroundColor()), kind: canvas.NewText("", theme.DisabledColor()), check: widget.NewCheck("", nil)}
	h.name.TextSize = 14
	h.name.TextStyle = fyne.TextStyle{Monospace: true}
	h.kind.TextSize = 10
	h.kind.TextStyle = fyne.TextStyle{Monospace: true}
	h.ExtendBaseWidget(h)
	return h
}

func (h *gridHeader) bind(col int) {
	h.column = col
	h.check.Hide()
	h.name.Show()
	h.kind.Show()
	h.kind.Text = ""
	h.kind.Color = theme.DisabledColor()
	switch {
	case col == 0:
		h.name.Hide()
		h.kind.Hide()
		h.check.Show()
		h.check.OnChanged = nil
		all := h.model.length() > 0
		for row := 0; row < h.model.length(); row++ {
			if !h.model.selected[row] {
				all = false
				break
			}
		}
		h.check.SetChecked(all)
		h.check.OnChanged = func(value bool) {
			if h.model.selectRow != nil {
				for row := 0; row < h.model.length(); row++ {
					h.model.selectRow(row, value)
				}
				h.table.Refresh()
			}
		}
	case col == 1:
		h.name.Text = "#"
	case col >= 2 && col < len(h.model.columns)+2:
		h.name.Text = h.model.columns[col-2].Name
		for _, c := range h.model.info.Columns {
			if c.Name == h.name.Text {
				h.kind.Text = c.Type
				if c.Key == "PRI" {
					h.kind.Color = color.NRGBA{R: 217, G: 119, B: 6, A: 255}
				}
				break
			}
		}
	}
	h.Refresh()
}
func (h *gridHeader) CreateRenderer() fyne.WidgetRenderer { return &gridHeaderRenderer{h: h} }

type gridHeaderRenderer struct{ h *gridHeader }

func (r *gridHeaderRenderer) MinSize() fyne.Size { return fyne.NewSize(28, 26) }
func (r *gridHeaderRenderer) Layout(size fyne.Size) {
	r.h.check.Resize(size)
	nameY := float32(6)
	if r.h.column == 1 {
		nameY = 12
	}
	r.h.name.Move(fyne.NewPos(6, nameY))
	r.h.name.Resize(fyne.NewSize(max(0, size.Width-12), 18))
	r.h.kind.Move(fyne.NewPos(6, 24))
	r.h.kind.Resize(fyne.NewSize(max(0, size.Width-12), 12))
	if r.h.column >= 2 && r.h.column < len(r.h.model.columns)+2 {
		r.h.name.Text = fitText(r.h.model.columns[r.h.column-2].Name, max(0, size.Width-12), r.h.name.TextSize, r.h.name.TextStyle)
	}
}
func (r *gridHeaderRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.h.name, r.h.kind, r.h.check}
}
func (r *gridHeaderRenderer) Refresh() {
	r.h.name.Color = theme.ForegroundColor()
	r.h.kind.Color = theme.DisabledColor()
	if r.h.column >= 2 && r.h.column < len(r.h.model.columns)+2 {
		for _, column := range r.h.model.info.Columns {
			if column.Name == r.h.model.columns[r.h.column-2].Name && column.Key == "PRI" {
				r.h.kind.Color = color.NRGBA{R: 217, G: 119, B: 6, A: 255}
				break
			}
		}
	}
	r.Layout(r.h.Size())
	r.h.name.Refresh()
	r.h.kind.Refresh()
	r.h.check.Refresh()
}
func (r *gridHeaderRenderer) Destroy() {}
