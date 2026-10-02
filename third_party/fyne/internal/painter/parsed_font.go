package painter

import (
	"bytes"
	"crypto/sha256"
	"sync"

	"github.com/go-text/typesetting/font"
)

// Theme scopes change colors independently, but often use identical font bytes.
// Retain a bounded set of parsed immutable faces across scopes and theme changes.
// A content key also detects a resource whose font bytes have changed.
const parsedFontCacheSize = 32
const scopedFontCacheSize = 4096

type parsedFontEntry struct {
	digest [sha256.Size]byte
	face   *font.Face
}

var parsedFonts struct {
	sync.Mutex
	entries [parsedFontCacheSize]parsedFontEntry
	next    int
}

var scopedFontMutex sync.Mutex
var scopedFontCount int

// ThemeOverride refreshes can allocate new scope IDs. Bound their small face
// maps as well; eviction is cheap because the parsed fonts remain shared.
func storeFontFace(key cacheID, value *FontCacheItem) {
	scopedFontMutex.Lock()
	defer scopedFontMutex.Unlock()
	if scopedFontCount >= scopedFontCacheSize {
		fontCache.Clear()
		scopedFontCount = 0
	}
	fontCache.Store(key, value)
	scopedFontCount++
}

func clearFontFaces() {
	scopedFontMutex.Lock()
	defer scopedFontMutex.Unlock()
	fontCache.Clear()
	scopedFontCount = 0
}

func parsedFont(content []byte) (*font.Face, error) {
	digest := sha256.Sum256(content)
	parsedFonts.Lock()
	defer parsedFonts.Unlock()
	for _, entry := range parsedFonts.entries {
		if entry.face != nil && entry.digest == digest {
			return entry.face, nil
		}
	}
	face, err := font.ParseTTF(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	parsedFonts.entries[parsedFonts.next] = parsedFontEntry{digest: digest, face: face}
	parsedFonts.next = (parsedFonts.next + 1) % parsedFontCacheSize
	return face, nil
}
