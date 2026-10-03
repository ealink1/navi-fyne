package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
	"github.com/ealink1/super-link/internal/sqlworkbench"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

// ImportRows confirms the entire mapping and content, then commits bounded batches.
// On interruption or error, completed batches remain and the result reports them.
func (e *Engine) ImportRows(ctx context.Context, id string, request domain.ImportRequest) (outcome domain.ImportResult, err error) {
	outcome.Total = len(request.Rows)
	p, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return outcome, err
	}
	if p.ReadOnly || p.Config.Protection.RestrictDataImport {
		return outcome, domain.ErrReadOnly
	}
	if p.Revision != request.Revision {
		return outcome, domain.ErrConflict
	}
	if err = e.authorizeImport(p, request); err != nil {
		return outcome, err
	}
	current, session, release, err := e.acquire(ctx, id, request.Object.Scope)
	if err != nil {
		return outcome, err
	}
	defer release()
	if current.Revision != p.Revision {
		return outcome, domain.ErrConflict
	}
	inspector, ok := session.client.(adapter.TableInspector)
	if !ok {
		return outcome, errors.New("driver has no table metadata capability")
	}
	writer, ok := session.client.(adapter.AtomicTableWriter)
	if !ok {
		return outcome, errors.New("driver has no atomic import capability")
	}
	name, err := sqlworkbench.ObjectName(p.SQLDialect(), request.Object)
	if err != nil {
		return outcome, err
	}
	if request.Object.Schema == "" {
		name = request.Object.Name
	}
	info, err := inspector.TableInfo(ctx, request.Object.Scope, name)
	if err != nil {
		return outcome, err
	}
	columns, err := importColumns(request, info)
	if err != nil {
		return outcome, err
	}
	// Validate every value before committing the first batch.
	for row, values := range request.Rows {
		if _, err = importRow(request, columns, row, values); err != nil {
			return outcome, err
		}
		if err = ctx.Err(); err != nil {
			return outcome, err
		}
	}
	start := time.Now()
	defer func() {
		auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		auditErr := e.Profiles.Store.AddHistory(auditCtx, domain.History{ProfileID: id, Text: "IMPORT " + request.Object.Name, Success: err == nil, Rows: outcome.Committed, Duration: time.Since(start)})
		if auditErr != nil && err == nil {
			err = &domain.CommittedWarning{Cause: auditErr}
		}
	}()
	for begin := 0; begin < len(request.Rows); begin += request.BatchSize {
		if err = e.checkRevision(ctx, id, request.Revision); err != nil {
			return outcome, err
		}
		end := min(begin+request.BatchSize, len(request.Rows))
		changes := domain.TableChanges{Object: request.Object, Revision: request.Revision, Import: true}
		for row := begin; row < end; row++ {
			values, _ := importRow(request, columns, row, request.Rows[row])
			changes.Rows = append(changes.Rows, domain.RowChange{Kind: "insert", Values: values})
		}
		statements, buildErr := sqlworkbench.BuildChanges(p.SQLDialect(), changes, info)
		if buildErr != nil {
			return outcome, buildErr
		}
		outcome.Attempted = true
		rows, writeErr := writeImportBatch(ctx, p, writer, statements)
		if writeErr != nil {
			discardFailedWriteSession(session, writeErr)
			return outcome, fmt.Errorf("import batch beginning at row %d failed; %d previously committed rows remain: %w", begin+1, outcome.Committed, writeErr)
		}
		outcome.Committed += rows
		outcome.Batches++
	}
	return outcome, nil
}

func writeImportBatch(ctx context.Context, p domain.Profile, writer adapter.AtomicTableWriter, statements []domain.Statement) (int64, error) {
	if p.Config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.Config.QueryTimeout)*time.Second)
		defer cancel()
	}
	return writer.ApplyStatements(ctx, statements)
}
func importColumns(request domain.ImportRequest, info domain.TableInfo) (map[int]connection.ColumnDefinition, error) {
	metadata := map[string]connection.ColumnDefinition{}
	for _, c := range info.Columns {
		metadata[c.Name] = c
	}
	columns := map[int]connection.ColumnDefinition{}
	seen := map[string]bool{}
	for index, name := range request.Mapping {
		column, ok := metadata[name]
		if !ok || index < 0 || index >= len(request.Columns) || seen[name] {
			return nil, errors.New("mapping contains an unknown or duplicate target column")
		}
		if !domain.WritableColumn(column) {
			return nil, errors.New("mapping cannot assign a computed column")
		}
		seen[name] = true
		columns[index] = column
	}
	return columns, nil
}
func importRow(request domain.ImportRequest, columns map[int]connection.ColumnDefinition, row int, source []any) (map[string]any, error) {
	if len(source) != len(request.Columns) {
		return nil, fmt.Errorf("import row %d has a mismatched field count", row+1)
	}
	values := make(map[string]any, len(columns))
	for index, column := range columns {
		value, err := sqlworkbench.ConvertValue(column.Type, source[index])
		if err != nil {
			return nil, fmt.Errorf("row %d column %s has invalid %s data: %w", row+1, column.Name, column.Type, err)
		}
		if value == nil && column.Nullable != "YES" {
			return nil, fmt.Errorf("row %d column %s does not allow NULL", row+1, column.Name)
		}
		values[column.Name] = value
	}
	return values, nil
}
