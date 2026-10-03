package sqlworkbench

import "github.com/ealink1/super-link/internal/domain"

// PreviewRead uses metadata's literal identifier components, including dots
// inside a table name. No UI path parsing or source dialect SQL is involved.
func PreviewRead(dialect string, object domain.Object) (string, error) {
	target, err := ObjectName(dialect, object)
	if err != nil {
		return "", err
	}
	switch dialect {
	case "sqlserver":
		return "SELECT TOP 100 * FROM " + target + ";", nil
	case "oracle", "dameng":
		return "SELECT * FROM " + target + " WHERE ROWNUM <= 100;", nil
	default:
		return "SELECT * FROM " + target + " LIMIT 100;", nil
	}
}
