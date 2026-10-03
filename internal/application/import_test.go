package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	adapter "github.com/ealink1/super-link/internal/infra/runtime"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

type importFixture struct {
	*tableFixture
	failAt  int
	applied []domain.Statement
}

func (f *importFixture) ApplyStatements(_ context.Context, s []domain.Statement) (int64, error) {
	f.writes++
	if f.writes == f.failAt {
		return 0, errors.New("fixture atomic batch rolled back")
	}
	f.applied = append(f.applied, s...)
	return int64(len(s)), nil
}
func importEngine(t *testing.T) (*Engine, domain.Profile, *importFixture, domain.ImportRequest) {
	t.Helper()
	e, p, table := tableEngine(t, false)
	table.info.Columns = []connection.ColumnDefinition{{Name: "id", Type: "INTEGER", Key: "PRI", Nullable: "NO"}, {Name: "name", Type: "TEXT", Nullable: "YES"}}
	f := &importFixture{tableFixture: table}
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return f, nil }
	r := domain.ImportRequest{Revision: p.Revision, Object: domain.Object{Name: "items", Kind: "table"}, Columns: []string{"id", "name"}, Rows: [][]any{{"1", "中文"}, {"2", nil}, {"3", "third"}}, Mapping: map[int]string{0: "id", 1: "name"}, BatchSize: 2}
	return e, p, f, r
}
func grantImport(t *testing.T, e *Engine, id string, r domain.ImportRequest) domain.ImportRequest {
	t.Helper()
	_, err := e.ImportRows(context.Background(), id, r)
	var required *domain.ConfirmationRequired
	if !errors.As(err, &required) {
		t.Fatal("missing confirmation", err)
	}
	r.Confirmation = required.Fingerprint
	return r
}
func TestImportValidatesEveryValueBeforeWritingAndBindsGrant(t *testing.T) {
	e, p, f, r := importEngine(t)
	r.Rows[2][0] = "not-an-integer"
	r = grantImport(t, e, p.ID, r)
	out, err := e.ImportRows(context.Background(), p.ID, r)
	if err == nil || out.Attempted || f.writes != 0 {
		t.Fatal("late invalid value partially imported", out, err)
	}
	r.Rows[2][0] = "3"
	r = grantImport(t, e, p.ID, r)
	r.Rows[0][1] = "changed after confirmation"
	_, err = e.ImportRows(context.Background(), p.ID, r)
	var required *domain.ConfirmationRequired
	if !errors.As(err, &required) || f.writes != 0 {
		t.Fatal("content changed without re-confirmation", err)
	}
	r.Confirmation = required.Fingerprint
	out, err = e.ImportRows(context.Background(), p.ID, r)
	if err != nil || out.Committed != 3 || out.Batches != 2 || !out.Attempted || f.applied[0].Args[0] != int64(1) {
		t.Fatal(out, err)
	}
	if _, err = e.ImportRows(context.Background(), p.ID, r); !errors.As(err, &required) || f.writes != 2 {
		t.Fatal("import grant replayed", err)
	}
}
func TestImportReportsPreviouslyCommittedBatchesOnFailure(t *testing.T) {
	e, p, f, r := importEngine(t)
	f.failAt = 2
	r = grantImport(t, e, p.ID, r)
	out, err := e.ImportRows(context.Background(), p.ID, r)
	if err == nil || out.Committed != 2 || out.Batches != 1 || !out.Attempted || !strings.Contains(err.Error(), "previously committed rows remain") {
		t.Fatal(out, err)
	}
	if len(f.applied) != 2 || f.closed.Load() != 1 {
		t.Fatal("failed session remained live")
	}
}
func TestImportProtectionIsIndependentFromEditingAndBlocksBeforeConnect(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		e, p, f, r := importEngine(t)
		p.Config.Protection = connection.ConnectionProtectionConfig{RestrictDataEdit: true, RestrictDataImport: blocked}
		var err error
		p, err = e.Profiles.Save(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		r.Revision = p.Revision
		if blocked {
			if _, err = e.ImportRows(context.Background(), p.ID, r); !errors.Is(err, domain.ErrReadOnly) || f.writes != 0 {
				t.Fatal(err)
			}
		} else {
			r = grantImport(t, e, p.ID, r)
			if out, err := e.ImportRows(context.Background(), p.ID, r); err != nil || out.Committed != 3 {
				t.Fatal(out, err)
			}
		}
	}
}
