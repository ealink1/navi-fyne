package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-text/typesetting/font"
)

// This build-time comparison uses pre-compression fonts kept outside Git.
// Regular CI independently pins every deployed font's bytes and coverage.
func TestCompressedFontsMatchEveryReferenceOutlineAndAdvance(t *testing.T) {
	reference := os.Getenv("NAVIFYNE_FONT_REFERENCE_DIR")
	target := os.Getenv("NAVIFYNE_FONT_CANDIDATE_DIR")
	if reference == "" {
		t.Skip("pre-compression reference directory is required for a font regeneration audit")
	}
	resources := map[string][]byte{fontResource.Name(): cjkFont, boldFontResource.Name(): cjkBoldFont, monoFontResource.Name(): monoFont}
	for name, candidate := range resources {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile(filepath.Join(reference, name))
			if err != nil {
				t.Fatal(err)
			}
			if target != "" {
				candidate, err = os.ReadFile(filepath.Join(target, name))
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := font.ParseTTF(bytes.NewReader(original))
			if err != nil {
				t.Fatal(err)
			}
			after, err := font.ParseTTF(bytes.NewReader(candidate))
			if err != nil {
				t.Fatal(err)
			}
			for gid := range 31036 {
				glyph := font.GID(gid)
				if after.HorizontalAdvance(glyph) != before.HorizontalAdvance(glyph) || !reflect.DeepEqual(after.GlyphData(glyph), before.GlyphData(glyph)) {
					t.Fatalf("go-text outline or advance changed at glyph %d", gid)
				}
			}
		})
	}
}
