package color_test

import (
	imagecolor "image/color"
	"testing"

	"github.com/stretchr/testify/assert"

	"fyne.io/fyne/v2/internal/color"
)

func Test_ToNRGBA_unmultiplyAlpha(t *testing.T) {
	for name, tt := range map[string]struct {
		color imagecolor.Color
		wantR uint8
		wantG uint8
		wantB uint8
		wantA uint8
	}{
		"RGBA": {
			color: imagecolor.RGBA{R: 100, G: 100, B: 100, A: 100},
			wantR: 255,
			wantG: 255,
			wantB: 255,
			wantA: 100,
		},
		"RGBA opaque": {
			color: imagecolor.RGBA{R: 100, G: 100, B: 100, A: 255},
			wantR: 100,
			wantG: 100,
			wantB: 100,
			wantA: 255,
		},
		"RGBA64": {
			color: imagecolor.RGBA64{R: 100<<8 + 123, G: 100<<8 + 123, B: 100<<8 + 123, A: 100<<8 + 123},
			wantR: 255,
			wantG: 255,
			wantB: 255,
			wantA: 100,
		},
		"RGBA64 opaque": {
			color: imagecolor.RGBA64{R: 100<<8 + 123, G: 100<<8 + 123, B: 100<<8 + 123, A: 255 << 8},
			wantR: 100,
			wantG: 100,
			wantB: 100,
			wantA: 255,
		},
		"custom": {
			color: customColor{r: 100<<8 + 123, g: 100<<8 + 123, b: 100<<8 + 123, a: 100<<8 + 123},
			wantR: 255,
			wantG: 255,
			wantB: 255,
			wantA: 100,
		},
		"custom opaque": {
			color: customColor{r: 100<<8 + 123, g: 100<<8 + 123, b: 100<<8 + 123, a: 255 << 8},
			wantR: 100,
			wantG: 100,
			wantB: 100,
			wantA: 255,
		},
	} {
		t.Run(name, func(t *testing.T) {
			gotR, gotG, gotB, gotA := color.ToNRGBA(tt.color)
			assert.Equal(t, tt.wantR, gotR)
			assert.Equal(t, tt.wantG, gotG)
			assert.Equal(t, tt.wantB, gotB)
			assert.Equal(t, tt.wantA, gotA)
		})
	}
}

type customColor struct {
	r, g, b, a uint32
}

var _ imagecolor.Color = customColor{}

func (c customColor) RGBA() (r, g, b, a uint32) {
	return c.r, c.g, c.b, c.a
}
