package sqlworkbench

import (
	"encoding/hex"
	"errors"
	"strings"
)

// ParsePath decodes the quoted PostgreSQL metadata path without splitting literal dots.
func ParsePath(path string) ([]string, error) {
	parts := make([]string, 0, 2)
	var part strings.Builder
	quoted := false
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '"' {
			if quoted && i+1 < len(path) && path[i+1] == '"' {
				part.WriteByte('"')
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if c == '.' && !quoted {
			if part.Len() == 0 {
				return nil, errors.New("empty metadata path component")
			}
			parts = append(parts, part.String())
			part.Reset()
			continue
		}
		part.WriteByte(c)
	}
	if quoted || part.Len() == 0 {
		return nil, errors.New("invalid quoted metadata path")
	}
	parts = append(parts, part.String())
	if len(parts) > 2 {
		return nil, errors.New("metadata path has too many components")
	}
	return parts, nil
}

// TextLiteral avoids MySQL backslash-mode ambiguity for structured filter values.
func TextLiteral(kind, value string) string {
	switch kind {
	case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks", "sphinx":
		return "CONVERT(X'" + hex.EncodeToString([]byte(value)) + "' USING utf8mb4)"
	}
	if kind == "postgres" || kind == "kingbase" || kind == "highgo" || kind == "vastbase" || kind == "opengauss" || kind == "gaussdb" {
		return "E'" + strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
