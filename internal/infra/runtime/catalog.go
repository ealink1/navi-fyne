package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/sqlworkbench"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

// sqlObjects distinguishes views and schema components without encoding SQL in the UI.
func (c *databaseClient) sqlObjects(ctx context.Context, scope string) ([]domain.Object, bool, error) {
	query, ok := c.session.(db.StatementQueryExecer)
	if !ok {
		return nil, false, nil
	}
	statement := ""
	switch c.descriptor.Key {
	case "sqlite":
		statement = "SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY type, name"
	case "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb":
		statement = "SELECT table_schema, table_name, table_type FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema') ORDER BY table_schema, table_type, table_name"
	case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks":
		if scope == "" {
			return nil, false, nil
		}
		statement = "SELECT TABLE_NAME AS table_name, TABLE_TYPE AS table_type FROM information_schema.tables WHERE TABLE_SCHEMA = " + sqlworkbench.TextLiteral(c.descriptor.Key, scope) + " ORDER BY TABLE_TYPE, TABLE_NAME"
	default:
		return nil, false, nil
	}
	rows, _, err := query.QueryContext(ctx, statement)
	if err != nil {
		return nil, true, err
	}
	objects := make([]domain.Object, 0, len(rows))
	for _, row := range rows {
		object := domain.Object{Scope: scope, Kind: "table"}
		if c.descriptor.Key == "sqlite" {
			object.Name = fmt.Sprint(row["name"])
			object.Kind = fmt.Sprint(row["type"])
		} else {
			object.Name = fmt.Sprint(row["table_name"])
			if schema := row["table_schema"]; schema != nil {
				object.Schema = fmt.Sprint(schema)
			}
			if strings.EqualFold(fmt.Sprint(row["table_type"]), "VIEW") {
				object.Kind = "view"
			}
		}
		objects = append(objects, object)
	}
	return objects, true, nil
}
