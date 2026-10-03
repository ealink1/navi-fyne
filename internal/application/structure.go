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

func (e *Engine) ApplyStructure(ctx context.Context, id string, request domain.StructureRequest) (result domain.StructureResult, err error) {
	p, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return result, err
	}
	if p.ReadOnly || p.Config.Protection.RestrictStructureEdit {
		return result, domain.ErrReadOnly
	}
	if p.Revision != request.Revision {
		return result, domain.ErrConflict
	}
	if len(request.Changes) == 0 || len(request.Changes) > 100 || len(request.BeforeHash) != 64 {
		return result, errors.New("invalid structure change request")
	}
	bound := request
	bound.Confirmation = ""
	raw, err := json.Marshal(bound)
	if err != nil || len(raw) > 1<<20 {
		return result, errors.New("invalid structure request or exceeds 1 MiB")
	}
	digest := sha256.Sum256(raw)
	if err = e.authorize(p, domain.Execution{Text: "structure:" + hex.EncodeToString(digest[:]), Scope: request.Object.Scope, Action: "structure", Confirmation: request.Confirmation}); err != nil {
		return result, err
	}
	if p.Config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.Config.QueryTimeout)*time.Second)
		defer cancel()
	}
	current, session, release, err := e.acquire(ctx, id, request.Object.Scope)
	if err != nil {
		return result, err
	}
	defer release()
	if current.Revision != request.Revision {
		return result, domain.ErrConflict
	}
	inspector, ok := session.client.(adapter.TableInspector)
	if !ok {
		return result, errors.New("driver has no table metadata capability")
	}
	writer, ok := session.client.(adapter.StructureWriter)
	if !ok {
		return result, errors.New("driver has no structure write capability")
	}
	name, err := sqlworkbench.ObjectName(p.SQLDialect(), request.Object)
	if err != nil {
		return result, err
	}
	if request.Object.Schema == "" {
		name = request.Object.Name
	}
	info, err := inspector.TableInfo(ctx, request.Object.Scope, name)
	if err != nil {
		return result, err
	}
	if domain.StructureHash(info) != request.BeforeHash {
		return result, errors.New("server structure changed; refresh the designer before saving")
	}
	statements, err := sqlworkbench.BuildStructure(p.SQLDialect(), request, info)
	if err != nil {
		return result, err
	}
	if err = e.checkRevision(ctx, id, request.Revision); err != nil {
		return result, err
	}
	start := time.Now()
	result, err = writer.ApplyStructure(ctx, statements)
	discardFailedWriteSession(session, err)
	auditCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	auditErr := e.Profiles.Store.AddHistory(auditCtx, domain.History{ProfileID: id, Text: "STRUCTURE " + request.Object.Name, Rows: int64(result.Applied), Success: err == nil, Duration: time.Since(start)})
	if auditErr != nil && err == nil {
		err = &domain.CommittedWarning{Cause: auditErr}
	}
	return result, err
}
