package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/upstream/db"
)

// AtomicTableWriter applies bounded parameterized statements in one transaction.
type AtomicTableWriter interface {
	ApplyStatements(context.Context, []domain.Statement) (int64, error)
}

func (c *databaseClient) ApplyStatements(ctx context.Context, statements []domain.Statement) (total int64, err error) {
	if len(statements) == 0 || len(statements) > 1000 {
		return 0, errors.New("invalid change batch size")
	}
	var executor db.StatementExecer
	var commit func() error
	var rollback func() error
	if c.oracleMode || c.descriptor.Key == "oracle" || c.descriptor.Key == "dameng" {
		provider, ok := c.database.(db.TransactionExecerProvider)
		if !ok {
			return 0, errors.New("driver does not support atomic table changes")
		}
		tx, openErr := provider.OpenTransactionExecer(ctx)
		if openErr != nil {
			return 0, openErr
		}
		executor, commit, rollback = tx, tx.Commit, tx.Rollback
		defer tx.Close()
	} else {
		if c.session == nil {
			return 0, errors.New("table changes require a pinned SQL session")
		}
		if _, ok := c.session.(db.StatementExecArgsExecer); !ok {
			return 0, errors.New("driver does not support bound parameters")
		}
		begin := "BEGIN"
		switch c.descriptor.Key {
		case "mysql", "goldendb", "mariadb", "oceanbase", "diros", "starrocks":
			begin = "START TRANSACTION"
		case "sqlserver":
			begin = "BEGIN TRANSACTION"
		case "sqlite", "postgres", "kingbase", "highgo", "vastbase", "opengauss", "gaussdb", "duckdb":
		default:
			return 0, errors.New("atomic table changes are not verified for this driver")
		}
		if _, err = c.session.ExecContext(ctx, begin); err != nil {
			return 0, err
		}
		executor = c.session
		commit = func() error { _, commitErr := executor.ExecContext(ctx, "COMMIT"); return commitErr }
		rollback = func() error {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, rollbackErr := executor.ExecContext(cleanup, "ROLLBACK")
			return rollbackErr
		}
	}
	completed := false
	defer func() {
		if !completed {
			if rollbackErr := rollback(); rollbackErr != nil {
				err = errors.Join(err, domain.ErrUnknownOutcome, fmt.Errorf("rollback could not be confirmed: %w", rollbackErr))
			} else if err != nil && !errors.Is(err, domain.ErrUnknownOutcome) {
				err = &domain.RolledBackError{Cause: err}
			}
		}
	}()
	bound, ok := executor.(db.StatementExecArgsExecer)
	if !ok {
		return 0, errors.New("transaction does not support bound parameters")
	}
	for i, s := range statements {
		rows, execErr := bound.ExecContextWithArgs(ctx, s.SQL, s.Args)
		if execErr != nil {
			return 0, fmt.Errorf("change %d failed: %w", i+1, execErr)
		}
		if s.RequireOne && rows != 1 {
			return 0, fmt.Errorf("change %d matched %d rows; server data changed or the locator is ambiguous", i+1, rows)
		}
		total += rows
	}
	if err = commit(); err != nil {
		return 0, errors.Join(domain.ErrUnknownOutcome, fmt.Errorf("commit failed: %w", err))
	}
	completed = true
	return total, nil
}

func (c *routedClient) ApplyStatements(ctx context.Context, statements []domain.Statement) (int64, error) {
	writer, ok := c.Client.(AtomicTableWriter)
	if !ok {
		return 0, errors.New("driver has no atomic table write capability")
	}
	return writer.ApplyStatements(ctx, statements)
}
