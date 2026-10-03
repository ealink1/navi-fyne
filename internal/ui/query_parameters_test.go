package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/ealink1/super-link/internal/domain"
)

func TestQueryParametersAndShortcutPreserveExecutedContext(t *testing.T) {
	w, p := parityWindow(t)
	s := w.openWorkspace(p, domain.Draft{ID: "params", Text: "SELECT :id AS id, '{first}-{second}' AS name", Scope: "main"})
	s.runAdaptive()
	if s.outputTabs.Selected() != s.parameterTab || len(s.lastResults) != 0 {
		t.Fatal("missing parameters executed")
	}
	if len(s.parameters) != 3 {
		t.Fatal("template parameters missing", len(s.parameters))
	}
	s.parameters["id"].typePicker.SetSelected("数值")
	s.parameters["id"].value.SetText("9223372036854775807")
	s.parameters["first"].value.SetText("prefix")
	s.parameters["second"].typePicker.SetSelected("NULL")
	s.code.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierSuper})
	waitUI(t, w)
	if len(s.lastResults) != 1 || s.lastRequest.Parameters["id"].Value != "9223372036854775807" || s.lastRequest.Scope != "main" || s.lastRequest.Revision != p.Revision {
		t.Fatal(s.status.Text, s.lastRequest)
	}
	s.editor.SetText("SELECT 99")
	s.scope.SetText("another")
	if s.lastRequest.Text != "SELECT :id AS id, '{first}-{second}' AS name" || s.lastRequest.Scope != "main" {
		t.Fatal("result provenance follows current editor")
	}
	drafts, err := w.Store.Drafts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range drafts {
		if d.Text == "9223372036854775807" {
			t.Fatal("parameter value persisted")
		}
	}
}
