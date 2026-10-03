package runtime

import (
	"context"
	"fmt"
	"strconv"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/db"
)

// TableInspector is optional because message and configuration clients have no SQL tables.
type TableInspector interface {
	TableInfo(context.Context, string, string) (domain.TableInfo, error)
}

func (c *databaseClient) TableInfo(ctx context.Context, scope, name string) (domain.TableInfo, error) {
	if c.descriptor.Key == "sqlite" && c.session != nil {
		return c.sqliteTableInfo(ctx, scope, name)
	}
	info := domain.TableInfo{}
	db.BindMetadataContext(c.database, ctx)
	defer db.ClearMetadataContext(c.database)
	var err error
	if c.descriptor.Key == "sqlite" && c.session != nil {
		info.Columns, err = c.sqliteColumns(ctx, name)
	} else {
		info.Columns, err = c.database.GetColumns(scope, name)
	}
	if err != nil {
		return info, fmt.Errorf("load table columns: %w", err)
	}
	info.DDL, err = c.Schema(ctx, scope, name)
	db.BindMetadataContext(c.database, ctx)
	if err != nil {
		info.Warnings = append(info.Warnings, "DDL: "+err.Error())
	}
	info.Indexes, err = c.database.GetIndexes(scope, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Indexes: "+err.Error())
	}
	info.ForeignKeys, err = c.database.GetForeignKeys(scope, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Foreign keys: "+err.Error())
	}
	info.Triggers, err = c.database.GetTriggers(scope, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Triggers: "+err.Error())
	}
	return info, nil
}

func (c *databaseClient) sqliteColumns(ctx context.Context, name string) ([]connection.ColumnDefinition, error) {
	quoted, err := sqlworkbench.Quote("sqlite", name)
	if err != nil {
		return nil, err
	}
	query, ok := c.session.(db.StatementQueryExecer)
	if !ok {
		return nil, fmt.Errorf("session has no metadata query capability")
	}
	rows, _, err := query.QueryContext(ctx, "PRAGMA table_xinfo("+quoted+")")
	if err != nil {
		return nil, err
	}
	columns := make([]connection.ColumnDefinition, 0, len(rows))
	for _, row := range rows {
		if fmt.Sprint(row["hidden"]) == "1" {
			continue
		}
		column := connection.ColumnDefinition{Name: fmt.Sprint(row["name"]), Type: fmt.Sprint(row["type"]), Nullable: "YES"}
		if h := fmt.Sprint(row["hidden"]); h == "2" || h == "3" {
			column.Extra = "generated"
		}
		if n, _ := strconv.Atoi(fmt.Sprint(row["pk"])); n > 0 {
			column.Key = "PRI"
			column.Nullable = "NO"
		}
		if fmt.Sprint(row["notnull"]) == "1" {
			column.Nullable = "NO"
		}
		if value := row["dflt_value"]; value != nil {
			text := fmt.Sprint(value)
			column.Default = &text
			column.HasDefault = true
		}
		columns = append(columns, column)
	}
	if len(columns) == 0 {
		return nil, domain.ErrNotFound
	}
	return columns, nil
}

func (c *routedClient) TableInfo(ctx context.Context, scope, name string) (domain.TableInfo, error) {
	if inspector, ok := c.Client.(TableInspector); ok {
		return inspector.TableInfo(ctx, scope, name)
	}
	return domain.TableInfo{}, fmt.Errorf("driver has no table metadata capability")
}

// splitPostgresObject preserves the upstream schema-qualified metadata convention.
func splitPostgresObject(name string) (domain.Object, error) {
	parts, err := sqlworkbench.ParsePath(name)
	if err != nil {
		return domain.Object{}, err
	}
	object := domain.Object{Name: parts[len(parts)-1], Kind: "table"}
	if len(parts) == 2 {
		object.Schema = parts[0]
	}
	return object, nil
}
