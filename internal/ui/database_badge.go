package ui

import (
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// Palette, frame radius and scale come from upstream DatabaseIcons.tsx.
var databaseColors = map[string]string{
	"mysql": "#00758F", "mariadb": "#003545", "oceanbase": "#0052CC", "postgres": "#336791", "redis": "#DC382D", "mongodb": "#47A248", "elasticsearch": "#FEC514", "kingbase": "#1890FF", "dameng": "#E6002D", "oracle": "#F80000", "sqlserver": "#CC2927", "clickhouse": "#FFBF00", "sqlite": "#003B57", "duckdb": "#FFC107", "vastbase": "#0066CC", "opengauss": "#2446A8", "gaussdb": "#0B7FAB", "goldendb": "#D97706", "highgo": "#00A86B", "iris": "#1F6FEB", "cache": "#1F6FEB", "tdengine": "#2962FF", "iotdb": "#0F766E", "rocketmq": "#EA580C", "mqtt": "#0EA5A4", "kafka": "#F97316", "rabbitmq": "#FF6B35", "pulsar": "#188FFF", "nacos": "#2E6BE6", "chroma": "#7C3AED", "qdrant": "#DC244C", "milvus": "#00A1EA", "diros": "#0050B3", "starrocks": "#00A6A6", "sphinx": "#2F5D62", "custom": "#888888",
}

func hexColor(text string) color.Color {
	value, err := strconv.ParseUint(strings.TrimPrefix(text, "#"), 16, 24)
	if err != nil {
		return color.NRGBA{R: 136, G: 136, B: 136, A: 255}
	}
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 255}
}
func databaseColor(kind string) color.Color { return hexColor(databaseColors[kind]) }

type databaseBadge struct {
	widget.BaseWidget
	name    string
	frame   *canvas.Rectangle
	image   *canvas.Image
	scale   float32
	maxSize float32
}

func newDatabaseBadge(name string) *databaseBadge {
	b := &databaseBadge{frame: canvas.NewRectangle(color.White), image: canvas.NewImageFromResource(icon("database")), maxSize: 20}
	b.ExtendBaseWidget(b)
	b.set(name)
	return b
}
func (b *databaseBadge) set(name string) {
	b.name = name
	b.image.Resource = icon(name)
	b.image.FillMode = canvas.ImageFillContain
	b.scale = 1
	b.frame.Hide()
	if strings.HasPrefix(name, "db-") {
		kind := strings.TrimPrefix(name, "db-")
		if kind == "cache" {
			b.image.Resource = icon("db-iris")
		}
		b.frame.Show()
		b.frame.FillColor = color.White
		b.frame.StrokeColor = databaseColor(kind)
		b.frame.StrokeWidth = 1.1
		b.frame.CornerRadius = 4
		b.scale = 0.64
		switch kind {
		case "oceanbase", "kingbase", "dameng", "oracle", "opengauss", "gaussdb", "goldendb", "highgo", "iris", "cache", "tdengine":
			b.scale = 0.72
		case "kafka", "pulsar":
			b.scale = 0.8
		case "rabbitmq", "qdrant", "milvus":
			b.scale = 0.74
		case "chroma":
			b.scale = 0.9
		case "vastbase":
			b.scale = 0.84
		case "iotdb":
			b.scale = 0.82
			b.frame.FillColor = hexColor("#0F766E")
		case "rocketmq", "mqtt", "starrocks":
			b.scale = 0.84
			b.frame.FillColor = hexColor("#0F172A")
		}
	}
	b.Refresh()
}
func (b *databaseBadge) CreateRenderer() fyne.WidgetRenderer { return &databaseBadgeRenderer{b: b} }

type databaseBadgeRenderer struct{ b *databaseBadge }

func (r *databaseBadgeRenderer) MinSize() fyne.Size { return fyne.NewSize(18, 18) }
func (r *databaseBadgeRenderer) Layout(size fyne.Size) {
	edge := min(size.Width, size.Height, r.b.maxSize)
	r.b.frame.Resize(fyne.NewSize(edge, edge))
	r.b.frame.Move(fyne.NewPos((size.Width-edge)/2, (size.Height-edge)/2))
	iconSize := edge * r.b.scale
	r.b.image.Move(fyne.NewPos((size.Width-iconSize)/2, (size.Height-iconSize)/2))
	r.b.image.Resize(fyne.NewSize(iconSize, iconSize))
}
func (r *databaseBadgeRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.frame, r.b.image}
}
func (r *databaseBadgeRenderer) Refresh() {
	r.Layout(r.b.Size())
	r.b.frame.Refresh()
	r.b.image.Refresh()
}
func (r *databaseBadgeRenderer) Destroy() {}
