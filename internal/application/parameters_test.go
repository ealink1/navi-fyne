package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/sqlparam"
)

func TestNamedParameterBindingAndConfirmationIncludeValues(t *testing.T) {
	e, p, client := testEngine(t, false)
	r := domain.Execution{Text: "UPDATE items SET name=:name WHERE id IN (:ids)", Write: true, Revision: p.Revision, Parameters: map[string]domain.Parameter{"name": {Type: "string", Value: "'; DROP TABLE items; --"}, "ids": {Type: "list", Value: []any{int64(9223372036854775807), int64(2)}}}}
	prepared, err := prepareExecution(p, r)
	if err != nil || prepared.Text != "UPDATE items SET name=? WHERE id IN (?,?)" || len(prepared.Args) != 3 || prepared.Args[1] != int64(9223372036854775807) {
		t.Fatal(prepared, err)
	}
	_, err = e.Execute(t.Context(), p.ID, r)
	var confirm *domain.ConfirmationRequired
	if !errors.As(err, &confirm) {
		t.Fatal(err)
	}
	r.Confirmation = confirm.Fingerprint
	r.Parameters["name"] = domain.Parameter{Type: "string", Value: "changed"}
	_, err = e.Execute(t.Context(), p.ID, r)
	if !errors.As(err, &confirm) || client.calls.Load() != 0 {
		t.Fatal("grant did not bind values", err)
	}
	r.Confirmation = confirm.Fingerprint
	if _, err = e.Execute(t.Context(), p.ID, r); err != nil {
		t.Fatal(err)
	}
	history, err := e.Profiles.Store.History(t.Context(), p.ID)
	if err != nil || len(history) == 0 || strings.Contains(history[0].Text, "changed") {
		t.Fatal("parameter value persisted in audit", err)
	}
}

func TestMissingInvalidAndOversizedParametersNeverConnect(t *testing.T) {
	e, p, f := testEngine(t, false)
	_, err := e.Execute(t.Context(), p.ID, domain.Execution{Text: "SELECT :id"})
	if !errors.Is(err, sqlparam.ErrMissingParameter) || f.calls.Load() != 0 {
		t.Fatal(err)
	}
	for _, r := range []domain.Execution{
		{Text: "SELECT :id", Parameters: map[string]domain.Parameter{"id": {Type: "list", Value: make([]any, 10001)}}},
		{Text: "SELECT :id", Parameters: map[string]domain.Parameter{"id": {Type: "number", Value: "NaN"}}},
		{Text: "SELECT :id", Parameters: map[string]domain.Parameter{"id": {Type: "number", Value: "null"}}},
		{Text: "SELECT ?", Args: []any{1}},
		{Text: "SELECT 1", Revision: p.Revision + 1},
		{Text: "DELETE FROM items", Write: true, Action: "structure"},
	} {
		if _, err = e.Execute(t.Context(), p.ID, r); err == nil || f.calls.Load() != 0 {
			t.Fatal("invalid request reached driver", err)
		}
	}
}

func TestDialectCommentsCannotHideWrites(t *testing.T) {
	for _, tc := range []struct {
		dialect, sql string
		write        bool
	}{
		{"postgres", "SELECT 1 # ; DELETE FROM items", true},
		{"mysql", "SELECT 1 # ; DELETE FROM items", false},
		{"mysql", "SELECT 1 --x; DELETE FROM items", true},
		{"postgres", "SELECT 1 --x; DELETE FROM items", false},
		{"postgres", `SELECT '\'; DELETE FROM items`, true},
		{"postgres", `SELECT E'\\\''; SELECT 1`, false},
	} {
		write, err := classifySQL(tc.sql, tc.dialect)
		if err != nil || write != tc.write {
			t.Fatal(tc, write, err)
		}
	}
}
