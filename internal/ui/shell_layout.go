package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

func shellInset(object fyne.CanvasObject, padding float32) *fyne.Container {
	return container.New(&shellInsetLayout{padding: padding}, object)
}

type shellInsetLayout struct{ padding float32 }

func shellRowInset(object fyne.CanvasObject) *fyne.Container {
	return container.New(&shellRowInsetLayout{}, object)
}

type shellRowInsetLayout struct{}

func (*shellRowInsetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return objects[0].MinSize().Add(fyne.NewSize(0, 16))
}
func (*shellRowInsetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(size.Width, max(0, size.Height-16)))
}

func (l *shellInsetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return objects[0].MinSize().Add(fyne.NewSquareSize(2 * l.padding))
}
func (l *shellInsetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.NewPos(l.padding, l.padding))
	objects[0].Resize(fyne.NewSize(max(0, size.Width-2*l.padding), max(0, size.Height-2*l.padding)))
}

func shellFixed(object fyne.CanvasObject, width, height float32) *fyne.Container {
	return container.New(&shellFixedLayout{width: width, height: height}, object)
}

type shellFixedLayout struct{ width, height float32 }

func (l *shellFixedLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	size := objects[0].MinSize()
	if l.width > 0 {
		size.Width = l.width
	}
	if l.height > 0 {
		size.Height = l.height
	}
	return size
}
func (l *shellFixedLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.Position{})
	objects[0].Resize(size)
}

func shellPanel(object fyne.CanvasObject, fill color.Color, radius, padding float32) *fyne.Container {
	background := shellRectangle(fill, radius, shellBorderColor)
	return container.NewStack(background, shellInset(object, padding))
}

// The reference's terminal welcome panel is centered and caps its width at 768.
// The desktop workspace keeps enough width for its three actions.
type shellWelcomeLayout struct{}

func (*shellWelcomeLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(700, 350) }
func (*shellWelcomeLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	width := min(768, max(0, size.Width-40))
	height := objects[0].MinSize().Height
	objects[0].Resize(fyne.NewSize(width, height))
	objects[0].Move(fyne.NewPos((size.Width-width)/2, max(20, (size.Height-height)/2)))
}
