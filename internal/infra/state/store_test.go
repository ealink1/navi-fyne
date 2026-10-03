package state

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func TestStoreRevisionAndCascadeWithReservedURICharacters(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "state # ? 中文.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p, err := store.SaveProfile(ctx, domain.Profile{ID: "p", Name: "Test", Config: connection.ConnectionConfig{Type: "sqlite"}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := store.SaveProfile(ctx, p); errs <- err }()
	}
	wg.Wait()
	close(errs)
	success, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("optimistic revision did not prevent overwrite")
	}
	if err = store.SaveDraft(ctx, domain.Draft{ID: "d", ProfileID: "p", Text: "SELECT 1"}); err != nil {
		t.Fatal(err)
	}
	if err = store.AddHistory(ctx, domain.History{ProfileID: "p", Text: "SELECT ?", Success: true}); err != nil {
		t.Fatal(err)
	}
	current, err := store.Profile(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteProfile(ctx, "p", current.Revision); err != nil {
		t.Fatal(err)
	}
	drafts, err := store.Drafts(ctx)
	if err != nil || len(drafts) != 0 {
		t.Fatal("orphaned drafts after connection deletion")
	}
	history, err := store.History(ctx, "p")
	if err != nil || len(history) != 0 {
		t.Fatal("orphaned history after connection deletion")
	}
}
