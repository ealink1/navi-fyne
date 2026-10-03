package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/db"
)

func (c *databaseClient) selectSchema(ctx context.Context, schema string) error {
	if schema == "" && !c.schemaSelected {
		return nil
	}
	switch c.descriptor.Key {
	case "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb":
		if c.session == nil {
			return errors.New("schema selection requires a pinned session")
		}
		query, ok := c.session.(db.StatementQueryExecer)
		if !ok {
			return errors.New("schema selection requires a metadata query capability")
		}
		if c.defaultSearchPath == "" {
			rows, _, err := query.QueryContext(ctx, "SHOW search_path")
			if err != nil {
				return fmt.Errorf("read session search path: %w", err)
			}
			if len(rows) == 0 {
				return errors.New("session search path is unavailable")
			}
			c.defaultSearchPath = fmt.Sprint(rows[0]["search_path"])
		}
		if schema == "" {
			_, _, err := query.QueryContext(ctx, "SELECT set_config('search_path', "+sqlworkbench.TextLiteral("postgres", c.defaultSearchPath)+", false)")
			if err == nil {
				c.schemaSelected = false
			}
			return err
		}
		name, err := sqlworkbench.Quote(c.descriptor.Key, schema)
		if err != nil {
			return err
		}
		_, err = c.session.ExecContext(ctx, "SET search_path TO "+name)
		if err == nil {
			c.schemaSelected = true
		}
		return err
	default:
		return errors.New("this driver does not accept a separate query schema")
	}
}
