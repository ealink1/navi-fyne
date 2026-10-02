package state

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
)

func TestSavedQueryMigrationReopenRevisionAndCascade(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.SaveProfile(ctx, domain.Profile{ID: "p", Name: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(ctx, domain.Draft{ID: "d", ProfileID: p.ID, Text: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.ExecContext(ctx, "DROP TABLE saved_queries; PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	query, err := store.SaveQuery(ctx, domain.SavedQuery{ID: "q", ProfileID: p.ID, Title: "报表", Scope: "main", Schema: "public", Text: "SELECT '中文';"})
	if err != nil || query.Revision != 1 {
		t.Fatal(query, err)
	}
	listed, err := store.SavedQueries(ctx, p.ID)
	if err != nil || len(listed) != 1 || listed[0].Text != "" {
		t.Fatal("listing should not load SQL payloads", err)
	}
	got, err := store.SavedQuery(ctx, query.ID)
	if err != nil || got.Text != query.Text || got.Schema != "public" {
		t.Fatal("document or context not preserved", err)
	}
	drafts, err := store.Drafts(ctx)
	if err != nil || len(drafts) != 1 || drafts[0].Text != "SELECT 1" {
		t.Fatal("migration lost an existing draft", err)
	}
	stale := query
	query.Text = "SELECT 2;"
	query, err = store.SaveQuery(ctx, query)
	if err != nil || query.Revision != 2 {
		t.Fatal(err)
	}
	if _, err = store.SaveQuery(ctx, stale); !errors.Is(err, domain.ErrQueryConflict) {
		t.Fatal("stale editor overwrote document", err)
	}
	if err = store.DeleteQuery(ctx, stale.ID, stale.Revision); !errors.Is(err, domain.ErrQueryConflict) {
		t.Fatal("stale delete removed newer document", err)
	}
	if err = store.DeleteProfile(ctx, p.ID, p.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SavedQuery(ctx, query.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("saved query survived connection deletion", err)
	}
}

func TestSavedQueryRejectsInvalidOrOversizedText(t *testing.T) {
	for _, text := range []string{"", "\xff", "SELECT\x00", strings.Repeat("x", 1<<20+1)} {
		if err := validateSavedQuery(domain.SavedQuery{ID: "q", ProfileID: "p", Title: "q", Text: text}); err == nil {
			t.Fatal("invalid text accepted")
		}
	}
}
