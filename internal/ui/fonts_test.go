package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/go-text/typesetting/font"
)

func TestBundledCFFFontsKeepCompleteCharacterCoverageAndAdvances(t *testing.T) {
	data, err := os.ReadFile("assets/fonts.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Fonts []struct {
			File     string             `json:"file"`
			Digest   string             `json:"sha256"`
			Count    int                `json:"codepoints"`
			Ranges   [][2]rune          `json:"ranges"`
			Advances map[string]float32 `json:"sample_advances"`
		} `json:"fonts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	resources := map[string][]byte{fontResource.Name(): cjkFont, boldFontResource.Name(): cjkBoldFont, monoFontResource.Name(): monoFont}
	if len(manifest.Fonts) != len(resources) {
		t.Fatal("font inventory does not cover every embedded resource")
	}
	for _, entry := range manifest.Fonts {
		t.Run(entry.File, func(t *testing.T) {
			content := resources[entry.File]
			digest := sha256.Sum256(content)
			if len(content) < 4 || string(content[:4]) != "OTTO" || hex.EncodeToString(digest[:]) != entry.Digest {
				t.Fatal("font format or reviewed bytes changed")
			}
			face, err := font.ParseTTF(bytes.NewReader(content))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, interval := range entry.Ranges {
				for value := interval[0]; value <= interval[1]; value++ {
					if _, ok := face.NominalGlyph(value); !ok {
						t.Fatalf("lost character U+%04X", value)
					}
					count++
				}
			}
			if count != entry.Count || count != 30890 {
				t.Fatal("font coverage was reduced", count)
			}
			for codepoint, want := range entry.Advances {
				value, err := strconv.ParseInt(codepoint, 10, 32)
				if err != nil {
					t.Fatal(err)
				}
				glyph, _ := face.NominalGlyph(rune(value))
				if got := face.HorizontalAdvance(glyph); got != want {
					t.Fatalf("U+%04X advance changed: %v != %v", value, got, want)
				}
			}
		})
	}
}
