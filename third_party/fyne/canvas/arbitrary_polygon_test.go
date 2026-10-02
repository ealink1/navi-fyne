package canvas_test

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestArbitraryPolygon_NewAndDefaults(t *testing.T) {
	points := []fyne.Position{
		{X: 0, Y: 0},
		{X: 100, Y: 0},
		{X: 100, Y: 100},
	}
	fill := color.White
	p := canvas.NewArbitraryPolygon(points, fill)

	assert.Equal(t, points, p.Points)
	assert.Equal(t, fill, p.FillColor)
	assert.Nil(t, p.StrokeColor)
	assert.Equal(t, float32(0), p.StrokeWidth)
}

func TestArbitraryPolygon_Properties(t *testing.T) {
	p := canvas.NewArbitraryPolygon(nil, color.Black)
	p.StrokeWidth = 2.0
	p.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	p.CornerRadii = []float32{5, 10, 5}

	assert.Equal(t, float32(2.0), p.StrokeWidth)
	assert.Equal(t, color.NRGBA{R: 255, G: 0, B: 0, A: 255}, p.StrokeColor)
	assert.Equal(t, []float32{5, 10, 5}, p.CornerRadii)
}

func TestArbitraryPolygon_RendersToMarkup(t *testing.T) {
	points := []fyne.Position{
		{X: 0, Y: 0},
		{X: 10, Y: 0},
		{X: 10, Y: 10},
	}
	p := canvas.NewArbitraryPolygon(points, color.White)
	p.Resize(fyne.NewSize(10, 10))

	test.AssertObjectRendersToMarkup(t, "arbitrary_polygon.xml", p)
}
