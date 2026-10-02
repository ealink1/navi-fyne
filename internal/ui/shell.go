package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
	"strings"
)

func action(label, name string, run func()) *widget.Button {
	var resource fyne.Resource
	if name != "" {
		resource = icon(name)
	}
	button := widget.NewButtonWithIcon(label, resource, run)
	button.Importance = widget.LowImportance
	return button
}

func (w *Window) buildShell() {
	w.sidebar = newNavigator(w)
	w.docHeader = container.NewHBox()
	w.docBody = container.NewStack()
	w.docHost = container.NewBorder(w.docHeader, nil, nil, nil, w.docBody)
	w.tabs.OnSelected = func(*container.TabItem) { w.syncDocuments() }
	w.tabs.OnUnselected = func(*container.TabItem) {}
	query := headerAction("新建查询", w.newSelectedQuery)
	connection := headerAction("新建连接", func() { w.editProfile(domain.Profile{}) })
	header := container.NewHBox(widget.NewLabelWithStyle("GoNavi", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), query, connection,
		headerAction("管理连接分组", w.groupManager), action("数据工作流", "", w.workflowMenu), action("SQL 工具", "", w.sqlTools), action("驱动管理", "", w.driverManager), action("关于", "", w.about),
		layout.NewSpacer(), action("", "insight", w.aiEntry), action("", "settings", w.settingsPage))
	header.Layout = &toolbarLayout{height: 34}
	main := container.NewHSplit(w.sidebar.content(), w.docHost)
	main.Offset = 0.18
	brandInset := container.NewGridWrap(fyne.NewSize(80, 30), layout.NewSpacer())
	header.Objects = append([]fyne.CanvasObject{brandInset}, header.Objects...)
	w.Window.SetContent(container.NewBorder(header, nil, nil, nil, main))
	w.syncDocuments()
}

func (w *Window) syncDocuments() {
	if w.docHeader == nil {
		return
	}
	selected := w.tabs.Selected()
	buttons := make([]fyne.CanvasObject, 0, len(w.tabs.Items))
	for _, item := range w.tabs.Items {
		title, subtitle := item.Text, ""
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
		buttons = append(buttons, newDocumentTab(title, subtitle, item == selected, func() { w.tabs.Select(item) }, func() { w.closeTab(item) }))
	}
	strip := container.NewHScroll(container.NewHBox(buttons...))
	strip.SetMinSize(fyne.NewSize(0, 48))
	w.docHeader.Objects = []fyne.CanvasObject{container.NewBorder(nil, nil, nil, container.NewHBox(action("", "add-row", w.newSelectedQuery), action("", "connection-menu", w.documentMenu)), strip)}
	w.docHeader.Layout = layout.NewMaxLayout()
	w.docHeader.Refresh()
	if selected != nil {
		w.docBody.Objects = []fyne.CanvasObject{selected.Content}
	} else {
		w.docBody.Objects = nil
	}
	w.docBody.Refresh()
}

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

func (t *documentTab) Tapped(*fyne.PointEvent) { t.selectTab() }
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
	close := widget.NewButtonWithIcon("", theme.CancelIcon(), t.closeTab)
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
