package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/connection"
	"github.com/ealink1/super-link/internal/upstream/db"
)

// SQLite has one pooled connection, held by our session. Metadata must use that
// session too; calling the database pool while it is pinned waits indefinitely.
func (c *databaseClient) sqliteTableInfo(ctx context.Context, scope, name string) (domain.TableInfo, error) {
	info := domain.TableInfo{}
	var err error
	info.Columns, err = c.sqliteColumns(ctx, name)
	if err != nil {
		return info, err
	}
	info.DDL, err = c.Schema(ctx, scope, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "DDL: "+err.Error())
	}
	info.Indexes, err = c.sqliteIndexes(ctx, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Indexes: "+err.Error())
	}
	info.ForeignKeys, err = c.sqliteForeignKeys(ctx, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Foreign keys: "+err.Error())
	}
	info.Triggers, err = c.sqliteTriggers(ctx, name)
	if err != nil {
		info.Warnings = append(info.Warnings, "Triggers: "+err.Error())
	}
	return info, ctx.Err()
}

func (c *databaseClient) sqlitePragma(ctx context.Context, pragma, name string) ([]map[string]any, error) {
	quoted, err := sqlworkbench.Quote("sqlite", name)
	if err != nil {
		return nil, err
	}
	query, ok := c.session.(db.StatementQueryExecer)
	if !ok {
		return nil, fmt.Errorf("session has no metadata query capability")
	}
	rows, _, err := query.QueryContext(ctx, "PRAGMA "+pragma+"("+quoted+")")
	return rows, err
}

func (c *databaseClient) sqliteIndexes(ctx context.Context, name string) ([]connection.IndexDefinition, error) {
	rows, err := c.sqlitePragma(ctx, "index_list", name)
	if err != nil {
		return nil, err
	}
	var indexes []connection.IndexDefinition
	for _, row := range rows {
		indexName := fmt.Sprint(row["name"])
		fields, err := c.sqlitePragma(ctx, "index_info", indexName)
		if err != nil {
			return indexes, err
		}
		unique, _ := strconv.Atoi(fmt.Sprint(row["unique"]))
		for _, field := range fields {
			column := "[expression]"
			if field["name"] != nil {
				column = fmt.Sprint(field["name"])
			}
			seq, _ := strconv.Atoi(fmt.Sprint(field["seqno"]))
			indexes = append(indexes, connection.IndexDefinition{Name: indexName, ColumnName: column, NonUnique: 1 - unique, SeqInIndex: seq + 1, IndexType: "BTREE"})
		}
	}
	return indexes, nil
}

func (c *databaseClient) sqliteForeignKeys(ctx context.Context, name string) ([]connection.ForeignKeyDefinition, error) {
	rows, err := c.sqlitePragma(ctx, "foreign_key_list", name)
	if err != nil {
		return nil, err
	}
	var keys []connection.ForeignKeyDefinition
	for _, row := range rows {
		key := fmt.Sprintf("fk_%s_%s", name, row["id"])
		refColumn := ""
		if row["to"] != nil {
			refColumn = fmt.Sprint(row["to"])
		}
		keys = append(keys, connection.ForeignKeyDefinition{Name: key, ConstraintName: key, ColumnName: fmt.Sprint(row["from"]), RefTableName: fmt.Sprint(row["table"]), RefColumnName: refColumn})
	}
	return keys, nil
}

var sqliteTriggerEvent = regexp.MustCompile(`(?i)\b(BEFORE|AFTER|INSTEAD\s+OF)\s+(INSERT|UPDATE|DELETE)\b`)

func (c *databaseClient) sqliteTriggers(ctx context.Context, name string) ([]connection.TriggerDefinition, error) {
	query, ok := c.session.(db.StatementQueryArgsExecer)
	if !ok {
		return nil, fmt.Errorf("session has no bound metadata query capability")
	}
	rows, _, err := query.QueryContextWithArgs(ctx, "SELECT name,sql FROM sqlite_master WHERE type='trigger' AND tbl_name=? ORDER BY name", []any{name})
	if err != nil {
		return nil, err
	}
	var triggers []connection.TriggerDefinition
	for _, row := range rows {
		trigger := connection.TriggerDefinition{Name: fmt.Sprint(row["name"]), Statement: fmt.Sprint(row["sql"])}
		if match := sqliteTriggerEvent.FindStringSubmatch(trigger.Statement); len(match) > 0 {
			trigger.Timing, trigger.Event = strings.ToUpper(match[1]), strings.ToUpper(match[2])
		}
		triggers = append(triggers, trigger)
	}
	return triggers, nil
}
