package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

func shellVBox(objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedVBoxLayout(0), objects...)
}
func shellHBox(objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedHBoxLayout(0), objects...)
}
func shellBorder(top, bottom, left, right fyne.CanvasObject, center ...fyne.CanvasObject) *fyne.Container {
	objects := append([]fyne.CanvasObject(nil), center...)
	for _, object := range []fyne.CanvasObject{top, bottom, left, right} {
		if object != nil {
			objects = append(objects, object)
		}
	}
	return container.New(&shellBorderLayout{top: top, bottom: bottom, left: left, right: right}, objects...)
}

type shellBorderLayout struct{ top, bottom, left, right fyne.CanvasObject }

func shellMinimum(object fyne.CanvasObject) fyne.Size {
	if object != nil && object.Visible() {
		return object.MinSize()
	}
	return fyne.Size{}
}
func (l *shellBorderLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	top, bottom, left, right := shellMinimum(l.top), shellMinimum(l.bottom), shellMinimum(l.left), shellMinimum(l.right)
	var center fyne.Size
	for _, object := range objects {
		if object != l.top && object != l.bottom && object != l.left && object != l.right {
			size := shellMinimum(object)
			center.Width = max(center.Width, size.Width)
			center.Height = max(center.Height, size.Height)
		}
	}
	return fyne.NewSize(max(top.Width, bottom.Width, left.Width+center.Width+right.Width), top.Height+bottom.Height+max(left.Height, right.Height, center.Height))
}
func (l *shellBorderLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	top, bottom, left, right := shellMinimum(l.top), shellMinimum(l.bottom), shellMinimum(l.left), shellMinimum(l.right)
	height := max(0, size.Height-top.Height-bottom.Height)
	place := func(object fyne.CanvasObject, pos fyne.Position, size fyne.Size) {
		if object != nil && object.Visible() {
			object.Move(pos)
			object.Resize(size)
		}
	}
	place(l.top, fyne.Position{}, fyne.NewSize(size.Width, top.Height))
	place(l.bottom, fyne.NewPos(0, size.Height-bottom.Height), fyne.NewSize(size.Width, bottom.Height))
	place(l.left, fyne.NewPos(0, top.Height), fyne.NewSize(left.Width, height))
	place(l.right, fyne.NewPos(size.Width-right.Width, top.Height), fyne.NewSize(right.Width, height))
	for _, object := range objects {
		if object != l.top && object != l.bottom && object != l.left && object != l.right {
			place(object, fyne.NewPos(left.Width, top.Height), fyne.NewSize(max(0, size.Width-left.Width-right.Width), height))
		}
	}
}
