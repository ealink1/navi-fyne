package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ealink1/super-link/internal/domain"
)

type StructureWriter interface {
	ApplyStructure(context.Context, []string) (domain.StructureResult, error)
}

// ApplyStructure reports MySQL's implicit commits separately from atomic DDL.
func (c *databaseClient) ApplyStructure(ctx context.Context, statements []string) (result domain.StructureResult, err error) {
	if c.session == nil || len(statements) == 0 || len(statements) > 500 {
		return result, errors.New("invalid structure batch or session")
	}
	switch c.descriptor.Key {
	case "sqlite", "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb":
		result.Atomic = true
	case "mysql", "mariadb", "goldendb", "oceanbase":
		if c.oracleMode {
			return result, errors.New("Oracle mode requires explicit DDL")
		}
	default:
		return result, errors.New("driver has no verified structured DDL capability")
	}
	if result.Atomic {
		if _, err = c.session.ExecContext(ctx, "BEGIN"); err != nil {
			return result, err
		}
		finished := false
		defer func() {
			if !finished {
				cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if _, rollbackErr := c.session.ExecContext(cleanup, "ROLLBACK"); rollbackErr != nil {
					err = errors.Join(err, domain.ErrUnknownOutcome, rollbackErr)
				} else if !errors.Is(err, domain.ErrUnknownOutcome) {
					result.Applied = 0
					if err != nil {
						err = &domain.RolledBackError{Cause: err}
					}
				}
			}
		}()
		for n, statement := range statements {
			result.Attempted = true
			if _, err = c.session.ExecContext(ctx, statement); err != nil {
				return result, fmt.Errorf("structure operation %d failed: %w", n+1, err)
			}
			result.Applied++
		}
		if _, err = c.session.ExecContext(ctx, "COMMIT"); err != nil {
			return result, errors.Join(domain.ErrUnknownOutcome, err)
		}
		finished = true
		return result, nil
	}
	for n, statement := range statements {
		result.Attempted = true
		if _, err = c.session.ExecContext(ctx, statement); err != nil {
			return result, errors.Join(domain.ErrUnknownOutcome, fmt.Errorf("structure operation %d failed; %d earlier operations remain: %w", n+1, result.Applied, err))
		}
		result.Applied++
	}
	return result, nil
}
func (c *routedClient) ApplyStructure(ctx context.Context, statements []string) (domain.StructureResult, error) {
	writer, ok := c.Client.(StructureWriter)
	if !ok {
		return domain.StructureResult{}, errors.New("driver has no structure write capability")
	}
	return writer.ApplyStructure(ctx, statements)
}
