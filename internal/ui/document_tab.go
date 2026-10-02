package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type documentTab struct {
	widget.BaseWidget
	title, subtitle     string
	selected            bool
	selectTab, closeTab func()
}

func newDocumentTab(title, subtitle string, selected bool, selectTab, closeTab func()) *documentTab {
	t := &documentTab{title: title, subtitle: subtitle, selected: selected, selectTab: selectTab, closeTab: closeTab}
	t.ExtendBaseWidget(t)
	return t
}

func (t *documentTab) Tapped(*fyne.PointEvent) {
	if t.selectTab != nil {
		t.selectTab()
	}
}
func (t *documentTab) close() {
	if t.closeTab != nil {
		t.closeTab()
	}
}
func (t *documentTab) CreateRenderer() fyne.WidgetRenderer {
	background := canvas.NewRectangle(theme.BackgroundColor())
	background.CornerRadius = 8
	background.StrokeColor = theme.InputBorderColor()
	background.StrokeWidth = 0.5
	top := canvas.NewRectangle(theme.PrimaryColor())
	if !t.selected {
		top.Hide()
	}
	title := canvas.NewText(t.title, theme.ForegroundColor())
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 13
	subtitle := canvas.NewText(t.subtitle, theme.ForegroundColor())
	subtitle.TextSize = 10.5
	close := widget.NewButtonWithIcon("", theme.CancelIcon(), t.close)
	close.Importance = widget.LowImportance
	return &documentTabRenderer{t: t, background: background, top: top, title: title, subtitle: subtitle, close: close}
}

type documentTabRenderer struct {
	t               *documentTab
	background, top *canvas.Rectangle
	title, subtitle *canvas.Text
	close           *widget.Button
}

func (r *documentTabRenderer) Layout(size fyne.Size) {
	r.background.Resize(size)
	r.top.Move(fyne.NewPos(0, 0))
	r.top.Resize(fyne.NewSize(size.Width, 2))
	r.title.Move(fyne.NewPos(9, 6))
	r.title.Resize(fyne.NewSize(size.Width-38, 19))
	r.title.Text = fitText(r.t.title, size.Width-38, r.title.TextSize, r.title.TextStyle)
	r.subtitle.Move(fyne.NewPos(9, 27))
	r.subtitle.Resize(fyne.NewSize(size.Width-18, 14))
	r.subtitle.Text = fitText(r.t.subtitle, size.Width-18, r.subtitle.TextSize, r.subtitle.TextStyle)
	r.close.Move(fyne.NewPos(size.Width-30, 8))
	r.close.Resize(fyne.NewSize(26, 26))
}
func (r *documentTabRenderer) MinSize() fyne.Size { return fyne.NewSize(166, 46) }
func (r *documentTabRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.background, r.top, r.title, r.subtitle, r.close}
}
func (r *documentTabRenderer) Refresh() {
	r.background.FillColor = theme.BackgroundColor()
	r.background.StrokeColor = theme.InputBorderColor()
	if r.t.selected {
		r.top.Show()
	} else {
		r.top.Hide()
	}
	r.top.FillColor = theme.PrimaryColor()
	r.title.Color = theme.ForegroundColor()
	r.subtitle.Color = theme.ForegroundColor()
	r.Layout(r.t.Size())
	for _, object := range r.Objects() {
		object.Refresh()
	}
	canvas.Refresh(r.t)
}
func (r *documentTabRenderer) Destroy() {}

func fitText(value string, width, size float32, style fyne.TextStyle) string {
	if fyne.MeasureText(value, size, style).Width <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && fyne.MeasureText(string(runes)+"…", size, style).Width > width {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimSpace(string(runes)) + "…"
}
