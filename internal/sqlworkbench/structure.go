package sqlworkbench

import (
	"errors"
	"reflect"
	"strings"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func structureDialect(key string) string {
	switch key {
	case "mysql", "mariadb", "goldendb", "oceanbase":
		return "mysql"
	case "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb":
		return "postgres"
	case "sqlite":
		return key
	}
	return ""
}

// BuildStructure validates structured changes; raw SQL stays in the query workflow.
func BuildStructure(dialect string, request domain.StructureRequest, info domain.TableInfo) ([]string, error) {
	kind := structureDialect(dialect)
	if kind == "" {
		return nil, errors.New("this driver does not support the structured designer")
	}
	if request.Object.Kind == "view" {
		return nil, errors.New("view definitions must be edited through DDL")
	}
	if len(request.Changes) == 0 || len(request.Changes) > 100 {
		return nil, errors.New("structure changes require 1–100 operations")
	}
	table, err := ObjectName(dialect, request.Object)
	if err != nil {
		return nil, err
	}
	columns := map[string]connection.ColumnDefinition{}
	for _, c := range info.Columns {
		columns[c.Name] = c
	}
	statements := []string{}
	indexes := append([]connection.IndexDefinition(nil), info.Indexes...)
	for _, change := range request.Changes {
		var next []string
		switch change.Kind {
		case "addColumn", "alterColumn", "dropColumn":
			next, err = buildColumnChange(kind, table, change, columns)
		case "addIndex", "dropIndex":
			next, err = buildIndexChange(kind, table, request.Object, change, columns, indexes)
			if err == nil {
				indexes = updateIndexes(indexes, change)
			}
		default:
			err = errors.New("unsupported structure operation")
		}
		if err != nil {
			return nil, err
		}
		statements = append(statements, next...)
	}
	if len(statements) == 0 {
		return nil, errors.New("no structure changes")
	}
	if len(strings.Join(statements, "\n")) > 1<<20 {
		return nil, errors.New("structure SQL exceeds 1 MiB")
	}
	return statements, nil
}

func buildColumnChange(kind, table string, change domain.StructureChange, columns map[string]connection.ColumnDefinition) ([]string, error) {
	old, exists := columns[change.OriginalName]
	if change.Kind != "addColumn" && !exists {
		return nil, errors.New("original column is no longer present")
	}
	oldName, err := Quote(kind, change.OriginalName)
	if change.Kind == "dropColumn" {
		if old.Key == "PRI" {
			return nil, errors.New("remove the primary-key constraint before dropping its column")
		}
		if err != nil {
			return nil, err
		}
		delete(columns, old.Name)
		return []string{"ALTER TABLE " + table + " DROP COLUMN " + oldName}, nil
	}
	c := change.Column
	if c.Nullable != "YES" && c.Nullable != "NO" {
		return nil, errors.New("invalid column nullability")
	}
	if kind == "sqlite" && c.Comment != "" {
		return nil, errors.New("SQLite column comments require explicit DDL")
	}
	name, err := Quote(kind, c.Name)
	if err != nil {
		return nil, err
	}
	if _, used := columns[c.Name]; used && (change.Kind == "addColumn" || c.Name != old.Name) {
		return nil, errors.New("duplicate column name")
	}
	if change.Kind == "addColumn" {
		if c.Key != "" || c.Extra != "" {
			return nil, errors.New("new primary or identity columns require explicit DDL")
		}
		definition, err := columnDefinition(kind, c, connection.ColumnDefinition{})
		if err != nil {
			return nil, err
		}
		columns[c.Name] = c
		statements := []string{"ALTER TABLE " + table + " ADD COLUMN " + definition}
		if kind == "postgres" && c.Comment != "" {
			statements = append(statements, "COMMENT ON COLUMN "+table+"."+name+" IS "+TextLiteral(kind, c.Comment))
		}
		return statements, nil
	}
	if c.Key != old.Key || c.Extra != old.Extra {
		return nil, errors.New("primary-key and identity changes require explicit DDL")
	}
	if kind != "mysql" && (c.Collation != old.Collation || c.Charset != old.Charset) {
		return nil, errors.New("charset and collation changes require explicit DDL")
	}
	statements := []string{}
	switch kind {
	case "mysql":
		definition, err := columnDefinition(kind, c, old)
		if err != nil {
			return nil, err
		}
		statements = append(statements, "ALTER TABLE "+table+" CHANGE COLUMN "+oldName+" "+definition)
	case "postgres":
		if c.Type != old.Type {
			if err := validateColumnType(c.Type); err != nil {
				return nil, err
			}
			statements = append(statements, "ALTER TABLE "+table+" ALTER COLUMN "+oldName+" TYPE "+c.Type)
		}
		if c.Nullable != old.Nullable {
			verb := "SET NOT NULL"
			if c.Nullable == "YES" {
				verb = "DROP NOT NULL"
			}
			statements = append(statements, "ALTER TABLE "+table+" ALTER COLUMN "+oldName+" "+verb)
		}
		if !reflect.DeepEqual(c.Default, old.Default) || c.HasDefault != old.HasDefault {
			verb := "DROP DEFAULT"
			if c.HasDefault && c.Default != nil {
				value, err := defaultExpression(kind, *c.Default)
				if err != nil {
					return nil, err
				}
				verb = "SET DEFAULT " + value
			}
			statements = append(statements, "ALTER TABLE "+table+" ALTER COLUMN "+oldName+" "+verb)
		}
		if c.Comment != old.Comment {
			statements = append(statements, "COMMENT ON COLUMN "+table+"."+oldName+" IS "+TextLiteral(kind, c.Comment))
		}
	case "sqlite":
		copy := c
		copy.Name = old.Name
		if !reflect.DeepEqual(copy, old) {
			return nil, errors.New("SQLite type, default and constraint edits require rebuilding the table; use explicit DDL")
		}
	}
	if kind != "mysql" && c.Name != old.Name {
		statements = append(statements, "ALTER TABLE "+table+" RENAME COLUMN "+oldName+" TO "+name)
	}
	delete(columns, old.Name)
	columns[c.Name] = c
	return statements, nil
}

func PreviewStructure(statements []string) string { return strings.Join(statements, ";\n\n") + ";" }

func qualifiedIndex(kind string, object domain.Object, name string) (string, error) {
	q, err := Quote(kind, name)
	if err != nil {
		return "", err
	}
	if kind == "postgres" && object.Schema != "" {
		schema, err := Quote(kind, object.Schema)
		if err != nil {
			return "", err
		}
		return schema + "." + q, nil
	}
	return q, nil
}
