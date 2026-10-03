package domain

import (
	"strings"

	"github.com/ealink1/super-link/internal/upstream/connection"
)

// WritableColumn distinguishes computed fields from server-generated defaults.
func WritableColumn(column connection.ColumnDefinition) bool {
	extra := strings.ToLower(column.Extra)
	extra = strings.ReplaceAll(extra, "default_generated", "")
	return !strings.Contains(extra, "generated") && !strings.Contains(extra, "computed") && !strings.Contains(extra, "identity always")
}
