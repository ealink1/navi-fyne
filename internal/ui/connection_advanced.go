package ui

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func (e *connectionEditor) advancedForm() fyne.CanvasObject {
	e.params = e.entry(e.original.Config.ConnectionParams, true)
	e.params.SetPlaceHolder("URI / DSN query 参数")
	protocol := widget.NewAccordion(widget.NewAccordionItem("协议扩展配置", e.extra))
	return container.NewPadded(container.NewVBox(widget.NewLabelWithStyle("额外连接参数", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), e.params, widget.NewLabel("认证密码请使用基本连接的密码字段。"), protocol))
}
func (e *connectionEditor) collectExtended(p *domain.Profile, cfg *connection.ConnectionConfig) error {
	var err error
	cfg.Timeout, err = parseTimeout(e.timeout.Text, 120)
	if err != nil {
		return err
	}
	cfg.QueryTimeout, err = parseTimeout(e.queryTimeout.Text, 3600)
	if err != nil {
		return err
	}
	cfg.ConnectionParams = e.params.Text
	cfg.Topology = e.topology.Selected
	cfg.Hosts = splitPatterns(e.hosts.Text)
	cfg.Protection = connection.ConnectionProtectionConfig{RestrictDataEdit: e.protectEdit.Checked, RestrictStructureEdit: e.protectStructure.Checked, RestrictScriptExecution: e.protectScript.Checked, RestrictDataImport: e.protectImport.Checked}
	p.DatabaseAllow = splitPatterns(e.allow.Text)
	p.DatabaseInclude = splitPatterns(e.include.Text)
	p.DatabaseExclude = splitPatterns(e.exclude.Text)
	p.IconType = e.iconType
	p.IconColor = e.iconColor
	return nil
}
func joinPatterns(values []string) string { return strings.Join(values, ", ") }
func splitPatterns(text string) []string {
	values := []string{}
	for _, part := range strings.Split(text, ",") {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func (e *connectionEditor) parseURI() {
	u, err := url.Parse(strings.TrimSpace(e.uri.Text))
	if err != nil || u.Host == "" {
		e.hint.SetText("连接串应包含协议与主机")
		return
	}
	e.host.SetText(u.Hostname())
	if u.Port() != "" {
		e.port.SetText(u.Port())
	}
	if u.User != nil {
		e.user.SetText(u.User.Username())
		if password, ok := u.User.Password(); ok {
			e.password.SetText(password)
		}
	}
	e.database.SetText(strings.TrimPrefix(u.Path, "/"))
	e.params.SetText(u.RawQuery)
	e.hint.SetText("已填充字段。连接串保持优先；使用手动字段时请清空连接串。")
}
func (e *connectionEditor) generateURI() {
	kind := "mysql"
	for _, d := range domain.Catalog() {
		if d.Name == e.kind.Selected {
			kind = d.Key
		}
	}
	if kind == "postgres" {
		kind = "postgresql"
	}
	host := e.host.Text
	if port, err := strconv.Atoi(e.port.Text); err == nil && port > 0 {
		host = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
	}
	u := url.URL{Scheme: kind, Host: host, Path: "/" + e.database.Text, RawQuery: e.params.Text}
	if e.user.Text != "" {
		u.User = url.UserPassword(e.user.Text, e.password.Text)
	}
	e.uri.SetText(u.String())
}

func (e *connectionEditor) appearanceForm() fyne.CanvasObject {
	e.iconType = e.original.IconType
	if e.iconType == "" {
		e.iconType = e.original.Config.Type
	}
	e.iconColor = e.original.IconColor
	preview := newDatabaseBadge("db-" + e.iconType)
	name := widget.NewLabelWithStyle(e.original.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	if name.Text == "" {
		name.SetText("连接名称")
	}
	update := func() {
		preview.set("db-" + e.iconType)
		if e.iconColor != "" {
			preview.frame.StrokeColor = hexColor(e.iconColor)
			preview.Refresh()
		}
	}
	icons := container.NewGridWithColumns(12)
	for _, d := range domain.Catalog() {
		button := widget.NewButtonWithIcon("", icon("db-"+d.Key), func() { e.iconType = d.Key; update() })
		button.Importance = widget.LowImportance
		icons.Add(button)
	}
	colors := container.NewHBox()
	for _, value := range []string{"#336791", "#003B57", "#DC382D", "#47A248", "#F80000", "#CC2927", "#1890FF", "#E6002D", "#FFBF00", "#2446A8", "#00A86B", "#0050B3", "#EA580C", "#7C3AED", "#0F766E"} {
		colors.Add(newColorSwatch(value, func() { e.iconColor = value; update() }))
	}
	update()
	return container.NewPadded(container.NewVBox(widget.NewLabel("图标"), icons, widget.NewLabel("颜色"), colors, widget.NewSeparator(), container.NewHBox(preview, container.NewVBox(name, widget.NewLabel("预览")))))
}

type colorSwatch struct {
	widget.BaseWidget
	value       string
	selectColor func()
}

func newColorSwatch(value string, selectColor func()) *colorSwatch {
	c := &colorSwatch{value: value, selectColor: selectColor}
	c.ExtendBaseWidget(c)
	return c
}
func (c *colorSwatch) Tapped(*fyne.PointEvent) { c.selectColor() }
func (c *colorSwatch) CreateRenderer() fyne.WidgetRenderer {
	circle := canvas.NewCircle(hexColor(c.value))
	circle.StrokeColor = theme.InputBorderColor()
	circle.StrokeWidth = 1
	return widget.NewSimpleRenderer(container.NewGridWrap(fyne.NewSize(22, 22), circle))
}
