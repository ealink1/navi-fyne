package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/ealink1/super-link/internal/domain"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
	"github.com/ealink1/super-link/internal/sqlworkbench"
)

// PreviewTableChanges validates staged rows against current server metadata.
func (e *Engine) PreviewTableChanges(ctx context.Context, id string, changes domain.TableChanges) ([]domain.Statement, error) {
	p, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Revision != changes.Revision {
		return nil, domain.ErrConflict
	}
	if p.ReadOnly || changes.Import && p.Config.Protection.RestrictDataImport || !changes.Import && p.Config.Protection.RestrictDataEdit {
		return nil, domain.ErrReadOnly
	}
	name, err := sqlworkbench.ObjectName(p.SQLDialect(), changes.Object)
	if err != nil {
		return nil, err
	}
	if changes.Object.Schema == "" {
		name = changes.Object.Name
	}
	info, err := e.TableInfo(ctx, id, changes.Object.Scope, name)
	if err != nil {
		return nil, err
	}
	return sqlworkbench.BuildChanges(p.SQLDialect(), changes, info)
}

// ApplyTableChanges requires single-use confirmation bound to values and target.
func (e *Engine) ApplyTableChanges(ctx context.Context, id string, changes domain.TableChanges) (int64, error) {
	p, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if p.ReadOnly || changes.Import && p.Config.Protection.RestrictDataImport || !changes.Import && p.Config.Protection.RestrictDataEdit {
		return 0, domain.ErrReadOnly
	}
	if p.Revision != changes.Revision {
		return 0, domain.ErrConflict
	}
	if changes.Object.Kind == "view" {
		return 0, errors.New("view changes require explicit SQL")
	}
	if p.Config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.Config.QueryTimeout)*time.Second)
		defer cancel()
	}
	bound := changes
	bound.Confirmation = ""
	raw, err := json.Marshal(bound)
	if err != nil || len(raw) > 16<<20 {
		return 0, errors.New("invalid or oversized staged changes")
	}
	digest := sha256.Sum256(raw)
	if err = e.authorize(p, domain.Execution{Text: "table-changes:" + hex.EncodeToString(digest[:]), Scope: changes.Object.Scope, Confirmation: changes.Confirmation}); err != nil {
		return 0, err
	}
	current, session, release, err := e.acquire(ctx, id, changes.Object.Scope)
	if err != nil {
		return 0, err
	}
	defer release()
	if current.Revision != changes.Revision {
		return 0, domain.ErrConflict
	}
	inspector, ok := session.client.(adapter.TableInspector)
	if !ok {
		return 0, errors.New("driver has no table metadata capability")
	}
	writer, ok := session.client.(adapter.AtomicTableWriter)
	if !ok {
		return 0, errors.New("driver has no atomic table write capability")
	}
	name, err := sqlworkbench.ObjectName(p.SQLDialect(), changes.Object)
	if err != nil {
		return 0, err
	}
	if changes.Object.Schema == "" {
		name = changes.Object.Name
	}
	info, err := inspector.TableInfo(ctx, changes.Object.Scope, name)
	if err != nil {
		return 0, err
	}
	statements, err := sqlworkbench.BuildChanges(p.SQLDialect(), changes, info)
	if err != nil {
		return 0, err
	}
	if err = e.checkRevision(ctx, id, changes.Revision); err != nil {
		return 0, err
	}
	start := time.Now()
	rows, writeErr := writer.ApplyStatements(ctx, statements)
	discardFailedWriteSession(session, writeErr)
	auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Bound row values stay in memory; history contains the operation only.
	text := "TABLE CHANGES " + changes.Object.Name
	auditErr := e.Profiles.Store.AddHistory(auditCtx, domain.History{ProfileID: id, Text: text, Success: writeErr == nil, Rows: rows, Duration: time.Since(start)})
	if auditErr != nil && writeErr == nil {
		return rows, &domain.CommittedWarning{Cause: auditErr}
	}
	return rows, writeErr
}
