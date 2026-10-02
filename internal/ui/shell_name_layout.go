package ui

import "fyne.io/fyne/v2"

type shellNameLayout struct{ text func() string }

func (l *shellNameLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	name := fyne.MeasureText(l.text(), 14, fyne.TextStyle{Bold: true})
	return fyne.NewSize(name.Width+objects[1].MinSize().Width+8, max(name.Height, objects[1].MinSize().Height))
}
func (l *shellNameLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	badge := objects[1].MinSize()
	width := max(0, min(fyne.MeasureText(l.text(), 14, fyne.TextStyle{Bold: true}).Width+8, size.Width-badge.Width-8))
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(width, size.Height))
	objects[1].Move(fyne.NewPos(width+8, max(0, (size.Height-badge.Height)/2)))
	objects[1].Resize(badge)
}
