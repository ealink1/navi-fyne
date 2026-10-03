package application

import (
	"context"
	"errors"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestFineGrainedProtectionBlocksBeforeConnecting(t *testing.T) {
	for _, test := range []struct {
		name       string
		protection connection.ConnectionProtectionConfig
		action     string
		table      bool
	}{
		{"script", connection.ConnectionProtectionConfig{RestrictScriptExecution: true}, "", false},
		{"structure", connection.ConnectionProtectionConfig{RestrictStructureEdit: true}, "structure", false},
		{"data edit", connection.ConnectionProtectionConfig{RestrictDataEdit: true}, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, p, f := tableEngine(t, false)
			p.Config.Protection = test.protection
			var err error
			p, err = e.Profiles.Save(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			if test.table {
				_, err = e.ApplyTableChanges(context.Background(), p.ID, domain.TableChanges{Revision: p.Revision, Rows: []domain.RowChange{{Kind: "insert", Values: map[string]any{"id": 1}}}})
			} else {
				text := "DELETE FROM items"
				if test.action == "structure" {
					text = "ALTER TABLE items ADD COLUMN note TEXT"
				}
				_, err = e.Execute(context.Background(), p.ID, domain.Execution{Text: text, Write: true, Action: test.action})
			}
			if !errors.Is(err, domain.ErrReadOnly) || f.calls.Load() != 0 || f.writes != 0 {
				t.Fatal("blocked operation reached driver", err)
			}
		})
	}
}
