package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

var sourceCategories = []string{"全部", "关系型数据库", "国产数据库", "NoSQL 数据库", "向量数据库", "时序数据库", "消息队列", "配置中心", "其他"}

func sourceCategory(d domain.Descriptor) string {
	switch d.Key {
	case "goldendb", "dameng", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb", "oceanbase":
		return "国产数据库"
	case "tdengine", "iotdb":
		return "时序数据库"
	case "custom":
		return "其他"
	}
	switch d.Family {
	case domain.SQL:
		return "关系型数据库"
	case domain.Cache, domain.Document, domain.Search:
		return "NoSQL 数据库"
	case domain.Vector:
		return "向量数据库"
	case domain.Message:
		return "消息队列"
	case domain.Configuration:
		return "配置中心"
	}
	return "其他"
}
func (w *Window) typePicker() {
	search := widget.NewEntry()
	search.SetPlaceHolder("搜索数据源类型")
	cards := container.NewGridWithColumns(2)
	scroll := container.NewVScroll(cards)
	category := "全部"
	selected := widget.NewLabelWithStyle("全部", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	var modal *widget.PopUp
	choose := func(d domain.Descriptor) {
		modal.Hide()
		w.editProfile(domain.Profile{ReadOnly: true, Config: connection.ConnectionConfig{Type: d.Key, Host: "localhost", Port: d.Port, User: "root", Timeout: 30, QueryTimeout: 30}})
	}
	refresh := func() {
		cards.Objects = nil
		needle := strings.ToLower(strings.TrimSpace(search.Text))
		for _, d := range domain.Catalog() {
			if category != "全部" && sourceCategory(d) != category || !strings.Contains(strings.ToLower(d.Name+" "+d.Key), needle) {
				continue
			}
			cards.Objects = append(cards.Objects, newSourceCard(d, func() { choose(d) }))
		}
		selected.SetText(fmt.Sprintf("%s · %d", category, len(cards.Objects)))
		cards.Refresh()
		scroll.ScrollToTop()
	}
	search.OnChanged = func(string) { refresh() }
	counts := map[string]int{}
	for _, d := range domain.Catalog() {
		counts[sourceCategory(d)]++
		counts["全部"]++
	}
	nav := widget.NewList(func() int { return len(sourceCategories) }, func() fyne.CanvasObject { return widget.NewLabel("") }, func(id int, item fyne.CanvasObject) {
		item.(*widget.Label).SetText(fmt.Sprintf("%s    %d", sourceCategories[id], counts[sourceCategories[id]]))
	})
	nav.OnSelected = func(id int) { category = sourceCategories[id]; refresh() }
	left := container.NewBorder(container.NewVBox(search, widget.NewLabel("分类")), nil, nil, nil, nav)
	split := container.NewHSplit(left, container.NewBorder(container.NewHBox(selected, layout.NewSpacer(), widget.NewLabel("单击进入配置表单")), nil, nil, nil, scroll))
	split.Offset = 0.25
	header := container.NewHBox(action("", "new-connection", nil), widget.NewLabelWithStyle("选择数据源类型", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), widget.NewLabel("1 选类型  ›  2 配参数  ›  3 测试保存"), action("", "stop", func() { modal.Hide() }))
	content := container.NewPadded(container.NewBorder(header, nil, nil, nil, split))
	modal = widget.NewModalPopUp(content, w.Window.Canvas())
	modal.Resize(fyne.NewSize(960, 750))
	modal.Show()
	nav.Select(0)
}

type sourceCard struct {
	widget.BaseWidget
	descriptor domain.Descriptor
	choose     func()
}

func newSourceCard(d domain.Descriptor, choose func()) *sourceCard {
	c := &sourceCard{descriptor: d, choose: choose}
	c.ExtendBaseWidget(c)
	return c
}
func (c *sourceCard) Tapped(*fyne.PointEvent) { c.choose() }
func (c *sourceCard) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.InputBackgroundColor())
	bg.StrokeColor = theme.InputBorderColor()
	bg.StrokeWidth = 0.7
	bg.CornerRadius = 10
	badge := newDatabaseBadge("db-" + c.descriptor.Key)
	badge.maxSize = 36
	title := canvas.NewText(c.descriptor.Name, theme.ForegroundColor())
	title.TextSize = 13
	title.TextStyle = fyne.TextStyle{Bold: true}
	sub := canvas.NewText("标准连接配置", theme.DisabledColor())
	sub.TextSize = 11
	if c.descriptor.Key == "sqlite" || c.descriptor.Key == "duckdb" {
		sub.Text = "本地文件连接"
	}
	category := canvas.NewText(sourceCategory(c.descriptor), theme.DisabledColor())
	category.TextSize = 10.5
	return &sourceCardRenderer{card: c, bg: bg, badge: badge, title: title, sub: sub, category: category}
}

type sourceCardRenderer struct {
	card                 *sourceCard
	bg                   *canvas.Rectangle
	badge                *databaseBadge
	title, sub, category *canvas.Text
}

func (r *sourceCardRenderer) MinSize() fyne.Size { return fyne.NewSize(240, 96) }
func (r *sourceCardRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.badge.Move(fyne.NewPos(14, 20))
	r.badge.Resize(fyne.NewSize(36, 36))
	r.title.Move(fyne.NewPos(66, 19))
	r.title.Text = fitText(r.card.descriptor.Name, size.Width-80, 13, r.title.TextStyle)
	r.sub.Move(fyne.NewPos(66, 40))
	r.category.Move(fyne.NewPos(14, 74))
}
func (r *sourceCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.badge, r.title, r.sub, r.category}
}
func (r *sourceCardRenderer) Refresh() { r.Layout(r.card.Size()); r.bg.Refresh(); r.title.Refresh() }
func (r *sourceCardRenderer) Destroy() {}
