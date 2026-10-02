package painter

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/go-text/typesetting/font"
)

type countedFontResource struct {
	fyne.Resource
	reads int
}

func (r *countedFontResource) Content() []byte {
	r.reads++
	return r.Resource.Content()
}

func TestCoveredTextDoesNotParseFallbackFonts(t *testing.T) {
	backup := &countedFontResource{Resource: theme.DefaultTextBoldFont()}
	symbol := &countedFontResource{Resource: theme.DefaultSymbolFont()}
	faces := lookupFaces(theme.DefaultTextFont(), backup, []fyne.Resource{nil, symbol}, "sans-serif", fyne.TextStyle{})
	for _, value := range "Welcome SELECT 123" {
		if _, ok := faces.ResolveFace(value).NominalGlyph(value); !ok {
			t.Fatal("covered text lost a glyph")
		}
	}
	if backup.reads != 0 || symbol.reads != 0 || len(faces.faces) != 1 {
		t.Fatal("covered text loaded unused backup fonts", backup.reads, symbol.reads)
	}
}

func TestMissingGlyphLoadsFallbacksOnceInOriginalPriority(t *testing.T) {
	// Simulate a primary face that lacks the requested character. Both fallback
	// fonts cover A; the first resource must win before locale/system fallback.
	backup := &countedFontResource{Resource: theme.DefaultTextBoldFont()}
	additional := &countedFontResource{Resource: theme.DefaultTextItalicFont()}
	faces := lookupFaces(theme.DefaultTextFont(), backup, []fyne.Resource{additional}, "sans-serif", fyne.TextStyle{})
	faces.faces = []*font.Face{}
	first := faces.ResolveFace('A')
	want := loadMeasureFont(backup.Resource)
	if first != want || faces.pendingLocale == nil {
		t.Fatal("fallback priority changed or system fallback ran unnecessarily")
	}
	for range 10 {
		if faces.ResolveFace('A') != want {
			t.Fatal("fallback selection changed on reuse")
		}
	}
	if backup.reads != 1 || additional.reads != 1 || faces.pendingFonts != nil {
		t.Fatal("fallbacks parsed more than once", backup.reads, additional.reads)
	}
}
