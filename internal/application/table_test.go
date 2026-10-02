package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

type tableFixture struct {
	*fakeClient
	info   domain.TableInfo
	read   int
	writes int
	last   []domain.Statement
	onRead func()
}

func (f *tableFixture) TableInfo(context.Context, string, string) (domain.TableInfo, error) {
	return f.info, nil
}
func (f *tableFixture) Execute(_ context.Context, request domain.Execution) ([]domain.Result, error) {
	f.read++
	if f.onRead != nil {
		f.onRead()
	}
	if strings.Contains(request.Text, "COUNT(*)") {
		return []domain.Result{{Rows: [][]any{{int64(125)}}}}, nil
	}
	return []domain.Result{{Columns: []domain.Column{{Name: "id"}, {Name: "name"}}, Rows: [][]any{{int64(1), "first"}}}}, nil
}
func (f *tableFixture) ApplyStatements(_ context.Context, s []domain.Statement) (int64, error) {
	f.writes++
	f.last = s
	return int64(len(s)), nil
}
func tableEngine(t *testing.T, readOnly bool) (*Engine, domain.Profile, *tableFixture) {
	e, p, fake := testEngine(t, readOnly)
	fixture := &tableFixture{fakeClient: fake, info: domain.TableInfo{Columns: []connection.ColumnDefinition{{Name: "id", Key: "PRI"}, {Name: "name"}}}}
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return fixture, nil }
	return e, p, fixture
}
func TestTablePageRejectsMutationAndStaleProfile(t *testing.T) {
	e, p, f := tableEngine(t, true)
	request := domain.TableRequest{Object: domain.Object{Name: "items", Scope: "main"}, Page: 2, Size: 100}
	page, err := e.TablePage(context.Background(), p.ID, request)
	if err != nil || page.Total != 125 || page.Page != 2 || f.read != 2 {
		t.Fatal(page, err, f.read)
	}
	request.Condition = "1=1); DELETE FROM items; --"
	if _, err = e.TablePage(context.Background(), p.ID, request); err == nil || f.read != 2 {
		t.Fatal("unsafe condition reached driver", err)
	}
	request.Condition = ""
	f.onRead = func() {
		f.onRead = nil
		p.Name = "changed"
		var saveErr error
		p, saveErr = e.Profiles.Save(context.Background(), p)
		if saveErr != nil {
			t.Fatal(saveErr)
		}
	}
	if _, err = e.TablePage(context.Background(), p.ID, request); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("mixed profile result returned", err)
	}
}
func TestTableChangesConfirmationBindsValuesTargetAndRevision(t *testing.T) {
	e, p, f := tableEngine(t, false)
	changes := domain.TableChanges{Revision: p.Revision, Object: domain.Object{Name: "items"}, Rows: []domain.RowChange{{Kind: "insert", Values: map[string]any{"id": 1, "name": "x"}}}}
	_, err := e.ApplyTableChanges(context.Background(), p.ID, changes)
	var required *domain.ConfirmationRequired
	if !errors.As(err, &required) || f.writes != 0 {
		t.Fatal("write before confirmation", err)
	}
	changes.Confirmation = required.Fingerprint
	changes.Rows[0].Values["name"] = "changed"
	if _, err = e.ApplyTableChanges(context.Background(), p.ID, changes); !errors.As(err, &required) || f.writes != 0 {
		t.Fatal("grant allowed changed values", err)
	}
	changes.Confirmation = required.Fingerprint
	if _, err = e.ApplyTableChanges(context.Background(), p.ID, changes); err != nil || f.writes != 1 {
		t.Fatal(err, f.writes)
	}
	if _, err = e.ApplyTableChanges(context.Background(), p.ID, changes); !errors.As(err, &required) || f.writes != 1 {
		t.Fatal("replayed grant", err)
	}
	p.ReadOnly = true
	var saveErr error
	p, saveErr = e.Profiles.Save(context.Background(), p)
	if saveErr != nil {
		t.Fatal(saveErr)
	}
	changes.Revision = p.Revision
	if _, err = e.ApplyTableChanges(context.Background(), p.ID, changes); !errors.Is(err, domain.ErrReadOnly) || f.writes != 1 {
		t.Fatal("read-only bypass", err)
	}
}
