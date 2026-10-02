package db

import "strings"

// Native results keep bytes separate from text. Display strings cannot be
// reconstructed safely: a BLOB containing the text "0x1234" is not hex data.
func nativeBinaryValue(value any, typeName string, preserve bool) ([]byte, bool) {
	if !preserve {
		return nil, false
	}
	data, ok := value.([]byte)
	if !ok {
		return nil, false
	}
	kind := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(typeName), " ", ""))
	if strings.Contains(kind, "BLOB") || strings.Contains(kind, "BINARY") || kind == "BYTEA" || kind == "RAW" || kind == "LONGRAW" || kind == "LONGVARRAW" || kind == "IMAGE" {
		if data == nil {
			return nil, true
		}
		return append([]byte{}, data...), true
	}
	return nil, false
}
