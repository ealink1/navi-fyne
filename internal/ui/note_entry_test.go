package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// Inspect actual rendered geometry: a positive theme value alone does not
// prove that the caret was laid out or that the border stayed invisible.
func TestNoteCaretVisibleWithoutInputOutline(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	w := app.NewWindow("caret")
	defer w.Close()
	for _, dark := range []bool{false, true} {
		app.Settings().SetTheme(Theme{Dark: dark})
		for _, multiline := range []bool{false, true} {
			e := newNoteEntry(multiline)
			entryTheme := noteEntryTheme{shellTheme: newShellTheme(), size: 14}
			w.SetContent(container.NewThemeOverride(e, entryTheme))
			w.Resize(fyne.NewSize(500, 200))
			w.Show()
			w.Canvas().Focus(e)
			test.Type(e, "中文 abc")
			if multiline {
				e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
				test.Type(e, "第二行")
			}
			e.Refresh()
			if w.Canvas().Focused() != e {
				t.Fatal("typing lost focus")
			}
			renderer := test.WidgetRenderer(e)
			for _, object := range renderer.Objects() {
				if rect, ok := object.(*canvas.Rectangle); ok && rect.Visible() && rect.StrokeWidth != 0 {
					t.Fatal("note input gained an outline")
				}
			}
			caret := noteTestCaret(e, entryTheme.Size(theme.SizeNameInputBorder))
			if caret == nil || !caret.Visible() || caret.Size().Height < 10 {
				t.Fatalf("caret missing: dark=%v multiline=%v", dark, multiline)
			}
			position := caret.Position()
			e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
			if caret.Position() == position {
				t.Fatal("caret did not follow keyboard navigation")
			}
			w.Canvas().Unfocus()
			if caret.Visible() {
				t.Fatal("caret stayed visible after losing focus")
			}
		}
	}
}

func noteTestCaret(object fyne.CanvasObject, width float32) *canvas.Rectangle {
	if rect, ok := object.(*canvas.Rectangle); ok && width > 0 && rect.Size().Width == width && rect.Size().Height >= 10 {
		return rect
	}
	var children []fyne.CanvasObject
	switch value := object.(type) {
	case *fyne.Container:
		children = value.Objects
	case fyne.Widget:
		children = test.WidgetRenderer(value).Objects()
	}
	for _, child := range children {
		if caret := noteTestCaret(child, width); caret != nil {
			return caret
		}
	}
	return nil
}
