package ui

import "fyne.io/fyne/v2"

// The conversation stays on the right without inflating the cached SQL/Shell/
// Note minimum widths. Smaller windows give the panel one third of the width.
type aiWorkspaceLayout struct{}

func (*aiWorkspaceLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	size := objects[0].MinSize()
	if objects[1].Visible() {
		size.Width = max(size.Width, 900)
		size.Height = max(size.Height, 500)
	}
	return size
}

func (*aiWorkspaceLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	width := float32(0)
	if objects[1].Visible() {
		width = min(380, size.Width*0.36)
		objects[1].Move(fyne.NewPos(size.Width-width, 0))
		objects[1].Resize(fyne.NewSize(width, size.Height))
	}
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(max(0, size.Width-width), size.Height))
}
