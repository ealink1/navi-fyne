package application

import (
	"context"
	"errors"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	adapter "github.com/ealink1/navi-fyne/internal/infra/runtime"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

type designerFixture struct{ *tableFixture }

func (f *designerFixture) ApplyStructure(_ context.Context, sql []string) (domain.StructureResult, error) {
	f.writes++
	return domain.StructureResult{Applied: len(sql), Atomic: true, Attempted: true}, nil
}
func TestStructureConfirmationBindsSchemaAndRejectsServerDrift(t *testing.T) {
	e, p, table := tableEngine(t, false)
	table.info.Columns = []connection.ColumnDefinition{{Name: "id", Type: "INTEGER", Key: "PRI", Nullable: "NO"}}
	f := &designerFixture{table}
	e.Factory = func(context.Context, domain.Profile) (adapter.Client, error) { return f, nil }
	r := domain.StructureRequest{Object: domain.Object{Name: "items"}, Revision: p.Revision, BeforeHash: domain.StructureHash(f.info), Changes: []domain.StructureChange{{Kind: "addColumn", Column: connection.ColumnDefinition{Name: "added", Type: "TEXT", Nullable: "YES"}}}}
	_, err := e.ApplyStructure(context.Background(), p.ID, r)
	var required *domain.ConfirmationRequired
	if !errors.As(err, &required) || f.writes != 0 {
		t.Fatal(err)
	}
	r.Confirmation = required.Fingerprint
	f.info.Columns = append(f.info.Columns, connection.ColumnDefinition{Name: "concurrent", Type: "TEXT", Nullable: "YES"})
	if _, err = e.ApplyStructure(context.Background(), p.ID, r); err == nil || f.writes != 0 {
		t.Fatal("stale designer changed server")
	}
	r.BeforeHash = domain.StructureHash(f.info)
	r.Confirmation = ""
	_, err = e.ApplyStructure(context.Background(), p.ID, r)
	if !errors.As(err, &required) {
		t.Fatal(err)
	}
	r.Confirmation = required.Fingerprint
	if result, err := e.ApplyStructure(context.Background(), p.ID, r); err != nil || result.Applied != 1 || f.writes != 1 {
		t.Fatal(result, err)
	}
	if _, err = e.ApplyStructure(context.Background(), p.ID, r); !errors.As(err, &required) || f.writes != 1 {
		t.Fatal("structure confirmation replayed", err)
	}
}
