package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ealink1/navi-fyne/internal/domain"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
	"github.com/ealink1/navi-fyne/internal/sqlworkbench"
	"github.com/ealink1/navi-fyne/internal/upstream/sqlaudit"
)

// TableInfo loads metadata on the same guarded connection lifecycle as queries.
func (e *Engine) TableInfo(ctx context.Context, id, scope, name string) (domain.TableInfo, error) {
	_, session, release, err := e.acquire(ctx, id, scope)
	if err != nil {
		return domain.TableInfo{}, err
	}
	defer release()
	inspector, ok := session.client.(adapter.TableInspector)
	if !ok {
		return domain.TableInfo{}, fmt.Errorf("driver has no table metadata capability")
	}
	return inspector.TableInfo(ctx, scope, name)
}

// TablePage holds one session lease across metadata, count and page queries.
func (e *Engine) TablePage(ctx context.Context, id string, request domain.TableRequest) (domain.TablePage, error) {
	page := domain.TablePage{Page: request.Page, Size: request.Size}
	if request.Revision != 0 {
		profile, err := e.Profiles.Get(ctx, id)
		if err != nil {
			return page, err
		}
		if profile.Revision != request.Revision {
			return page, domain.ErrConflict
		}
	}
	p, session, release, err := e.acquire(ctx, id, request.Object.Scope)
	if err != nil {
		return page, err
	}
	defer release()
	if request.Revision != 0 && p.Revision != request.Revision {
		return page, domain.ErrConflict
	}
	if p.Config.QueryTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(p.Config.QueryTimeout)*time.Second)
		defer cancel()
	}
	page.Revision = p.Revision
	name, err := sqlworkbench.ObjectName(p.SQLDialect(), request.Object)
	if err != nil {
		return page, err
	}
	metadataName := request.Object.Name
	if request.Object.Schema != "" {
		metadataName = name
	}
	inspector, ok := session.client.(adapter.TableInspector)
	if !ok {
		return page, errors.New("driver has no table metadata capability")
	}
	page.Info, err = inspector.TableInfo(ctx, request.Object.Scope, metadataName)
	if err != nil {
		return page, err
	}
	query, count, err := sqlworkbench.BuildPage(p.SQLDialect(), request, page.Info)
	if err != nil {
		return page, err
	}
	descriptor, _ := domain.Resolve(p.Config.Type)
	descriptor.Key = p.SQLDialect()
	write, err := Classify(descriptor, domain.Execution{Text: query})
	if err != nil {
		return page, err
	}
	if write {
		return page, errors.New("table conditions must be read-only")
	}
	if err = e.checkRevision(ctx, id, p.Revision); err != nil {
		return page, err
	}
	start := time.Now()
	results, err := session.client.Execute(ctx, domain.Execution{Scope: request.Object.Scope, Text: count, MaxRows: 1})
	if err != nil {
		return page, e.discardTableTransport(session, err)
	}
	if len(results) == 0 || len(results[0].Rows) == 0 || len(results[0].Rows[0]) == 0 {
		return page, fmt.Errorf("count returned no result")
	}
	page.Total, err = strconv.ParseInt(fmt.Sprint(results[0].Rows[0][0]), 10, 64)
	if err != nil || page.Total < 0 {
		return page, errors.New("invalid table count")
	}
	results, err = session.client.Execute(ctx, domain.Execution{Scope: request.Object.Scope, Text: query, MaxRows: request.Size})
	if err != nil {
		return page, e.discardTableTransport(session, err)
	}
	if err = e.checkRevision(ctx, id, p.Revision); err != nil {
		return domain.TablePage{}, err
	}
	if len(results) > 0 {
		page.Result = results[0]
	}
	page.Result.Duration = time.Since(start)
	historyCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err = e.Profiles.Store.AddHistory(historyCtx, domain.History{ProfileID: id, Text: sqlaudit.RedactQuery(descriptor.Key, query), Success: true, Rows: int64(len(page.Result.Rows)), Duration: page.Result.Duration}); err != nil {
		page.Result.Messages = append(page.Result.Messages, "Local history could not be saved.")
	}
	return page, nil
}

func (e *Engine) checkRevision(ctx context.Context, id string, revision int64) error {
	current, err := e.Profiles.Get(ctx, id)
	if err != nil {
		return err
	}
	if current.Revision != revision {
		return domain.ErrConflict
	}
	return nil
}

func (e *Engine) discardTableTransport(session *session, err error) error {
	_ = session.client.Close()
	session.client = nil
	return err
}
