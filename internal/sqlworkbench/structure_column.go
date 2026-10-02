package sqlworkbench

import (
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

var columnTypePattern = regexp.MustCompile(`(?i)^[a-z_][a-z0-9_]*(?:\s+(?:varying|precision|with|without|time|zone))*(?:\([0-9]+(?:\s*,\s*[0-9]+)?\))?(?:\s+unsigned)?(?:\[\])?$`)
var simpleSQLName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func validateColumnType(value string) error {
	if len(value) > 128 || !columnTypePattern.MatchString(value) {
		return errors.New("unsupported column type; use explicit DDL for expressions or enum definitions")
	}
	return nil
}
func columnDefinition(kind string, c, old connection.ColumnDefinition) (string, error) {
	name, err := Quote(kind, c.Name)
	if err != nil {
		return "", err
	}
	if err = validateColumnType(c.Type); err != nil {
		return "", err
	}
	definition := name + " " + c.Type
	if kind == "mysql" {
		extra := strings.TrimSpace(strings.ReplaceAll(strings.ToLower(c.Extra), "default_generated", ""))
		if extra != "" && extra != "auto_increment" {
			return "", errors.New("generated or ON UPDATE columns require explicit DDL")
		}
		for _, attribute := range []struct{ keyword, value string }{{"CHARACTER SET", c.Charset}, {"COLLATE", c.Collation}} {
			if attribute.value != "" {
				if !simpleSQLName.MatchString(attribute.value) {
					return "", errors.New("invalid charset or collation")
				}
				definition += " " + attribute.keyword + " " + attribute.value
			}
		}
	}
	if c.Nullable != "YES" && c.Nullable != "NO" {
		return "", errors.New("column nullability must be YES or NO")
	}
	if c.Nullable == "NO" {
		definition += " NOT NULL"
	}
	if c.HasDefault && c.Default != nil {
		value := *c.Default
		if old.Name == "" || !reflect.DeepEqual(c.Default, old.Default) || c.HasDefault != old.HasDefault {
			value, err = defaultExpression(kind, value)
			if err != nil {
				return "", err
			}
		}
		definition += " DEFAULT " + value
	}
	if kind == "mysql" {
		if strings.Contains(strings.ToLower(c.Extra), "auto_increment") {
			definition += " AUTO_INCREMENT"
		}
		if c.Comment != "" {
			definition += " COMMENT " + TextLiteral(kind, c.Comment)
		}
	}
	return definition, nil
}
func defaultExpression(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	upper := strings.ToUpper(value)
	switch upper {
	case "NULL", "TRUE", "FALSE", "CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP()", "CURRENT_DATE", "CURRENT_TIME":
		return upper, nil
	}
	if len(value) > 0 && (value[0] == '-' || value[0] >= '0' && value[0] <= '9') && json.Valid([]byte(value)) {
		return value, nil
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		inner := value[1 : len(value)-1]
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\'' {
				if i+1 >= len(inner) || inner[i+1] != '\'' {
					return "", errors.New("invalid default string")
				}
				i++
			}
		}
		return TextLiteral(kind, strings.ReplaceAll(inner, "''", "'")), nil
	}
	return "", errors.New("default requires a number, quoted string, NULL or a supported timestamp expression")
}
