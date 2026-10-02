package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/navi-fyne/internal/domain"
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
	w.buildDocuments()
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
