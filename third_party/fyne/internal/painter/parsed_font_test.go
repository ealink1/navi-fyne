package painter

import (
	"bytes"
	"fmt"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/go-text/typesetting/font"
)

func TestParsedFontSharesIdenticalContentAcrossScopesAndThemeChanges(t *testing.T) {
	content := theme.DefaultTextFont().Content()
	first, err := parsedFont(content)
	if err != nil {
		t.Fatal(err)
	}
	ClearFontCache()
	second, err := parsedFont(bytes.Clone(content))
	if err != nil || first != second {
		t.Fatal("identical bytes caused another font parse", err)
	}
	bold, err := parsedFont(theme.DefaultTextBoldFont().Content())
	if err != nil || bold == first {
		t.Fatal("different font faces were merged", err)
	}
}

func TestParsedFontConcurrentRequestsShareFace(t *testing.T) {
	content := theme.DefaultTextMonospaceFont().Content()
	const workers = 16
	results := make(chan *font.Face, workers)
	errors := make(chan error, workers)
	var pending sync.WaitGroup
	for range workers {
		pending.Add(1)
		go func() {
			defer pending.Done()
			face, err := parsedFont(content)
			if err != nil {
				errors <- err
				return
			}
			results <- face
		}()
	}
	pending.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var first *font.Face
	for result := range results {
		if first == nil {
			first = result
		} else if first != result {
			t.Fatal("concurrent calls parsed duplicate faces")
		}
	}
}

func TestParsedFontRejectsMalformedContentWithoutCachingFailure(t *testing.T) {
	for range 2 {
		if face, err := parsedFont([]byte("invalid font")); err == nil || face != nil {
			t.Fatal("malformed font was accepted")
		}
	}
}

func TestParsedFontEvictsOldContentAndDetectsChangedBytes(t *testing.T) {
	content := theme.DefaultTextFont().Content()
	firstContent := append(bytes.Clone(content), []byte("eviction-first")...)
	first, err := parsedFont(firstContent)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parsedFontCacheSize {
		changed := append(bytes.Clone(content), []byte(fmt.Sprintf("eviction-%d", i))...)
		face, err := parsedFont(changed)
		if err != nil || face == first {
			t.Fatal("changed bytes reused a previous resource", err)
		}
	}
	reloaded, err := parsedFont(firstContent)
	if err != nil || reloaded == first {
		t.Fatal("old parsed face was not evicted", err)
	}
}

func TestScopedFontCacheEvictsRefreshScopeIDs(t *testing.T) {
	clearFontFaces()
	t.Cleanup(clearFontFaces)
	first := cacheID{scope: "scope-0"}
	for i := 0; i <= scopedFontCacheSize; i++ {
		storeFontFace(cacheID{scope: fmt.Sprintf("scope-%d", i)}, &FontCacheItem{})
	}
	if _, ok := fontCache.Load(first); ok {
		t.Fatal("unlimited obsolete theme scopes retained")
	}
	if _, ok := fontCache.Load(cacheID{scope: fmt.Sprintf("scope-%d", scopedFontCacheSize)}); !ok {
		t.Fatal("most recent theme scope was evicted")
	}
}

func TestBundledGlyphsDoNotLoadTheLocaleFallback(t *testing.T) {
	resource := theme.DefaultTextFont()
	faces := lookupFaces(resource, resource, nil, "sans-serif", fyne.TextStyle{})
	if faces.pendingLocale == nil {
		t.Fatal("locale font was eagerly loaded")
	}
	face := faces.ResolveFace('A')
	if _, ok := face.NominalGlyph('A'); !ok || faces.pendingLocale == nil {
		t.Fatal("bundled glyph triggered a system-font load")
	}
	faces.ResolveFace('\U0010ffff')
	if faces.pendingLocale != nil {
		t.Fatal("missing glyph did not attempt the locale fallback")
	}
}
