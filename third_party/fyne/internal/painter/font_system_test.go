//go:build !ci && !test

package painter_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/painter"
	"github.com/stretchr/testify/assert"
)

func TestHangul(t *testing.T) {
	got := painter.CachedFontFace(fyne.TextStyle{}, nil, nil)
	f := got.Fonts.ResolveFace('안')
	gid, ok := f.Cmap.Lookup('안')
	assert.True(t, ok)
	assert.NotZero(t, gid)
}
