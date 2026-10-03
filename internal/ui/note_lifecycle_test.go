package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ealink1/super-link/internal/application"
	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/secrets"
)

type delayedNoteVault struct {
	secrets.Vault
	entered, release chan struct{}
}

func (v *delayedNoteVault) Put(raw []byte, persist bool) (string, error) {
	select {
	case v.entered <- struct{}{}:
	default:
	}
	<-v.release
	return v.Vault.Put(raw, persist)
}
func TestNoteSaveCoalescesEditsDuringPersistence(t *testing.T) {
	w, n := noteTestWindow(t)
	gate := &delayedNoteVault{Vault: n.service.Vault, entered: make(chan struct{}, 1), release: make(chan struct{})}
	n.service.Vault = gate
	n.newNote()
	n.editor.SetText("first")
	n.save()
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("save never started")
	}
	n.editor.SetText("latest during save")
	close(gate.release)
	pumpShell(t, w, func() bool { return !n.saving && n.savedRevision == n.editRevision })
	loaded, err := n.service.Load(context.Background())
	if err != nil || loaded.Notes[0].Body != "latest during save" {
		t.Fatal("new edit overwritten by callback", err)
	}
}
func TestNoteShutdownRebasesUndeliveredOwnCommit(t *testing.T) {
	w, n := noteTestWindow(t)
	gate := &delayedNoteVault{Vault: n.service.Vault, entered: make(chan struct{}, 1), release: make(chan struct{})}
	n.service.Vault = gate
	n.newNote()
	n.editor.SetText("first")
	n.save()
	select {
	case <-gate.entered:
	case <-time.After(time.Second):
		t.Fatal("save never started")
	}
	n.editor.SetText("last before close")
	flush := w.noteShutdownSnapshot()
	w.jobs.closing.Store(true)
	w.jobs.cancel()
	close(gate.release)
	w.jobs.wg.Wait()
	// The task context was cancelled before publication; the final snapshot still
	// must save. A commit without a delivered callback is simulated below too.
	if err := flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	n.book.Revision = n.service.CommittedRevision()
	n.editor.SetText("next last edit")
	committed, err := n.service.Save(context.Background(), n.book.Clone())
	if err != nil {
		t.Fatal(err)
	}
	if committed.Book.Revision == n.book.Revision {
		t.Fatal("fixture did not produce pending revision")
	}
	flush = w.noteShutdownSnapshot()
	if err := flush(context.Background()); err != nil {
		t.Fatal("undelivered own commit blocked flush", err)
	}
	loaded, err := n.service.Load(context.Background())
	if err != nil || loaded.Notes[0].Body != "next last edit" {
		t.Fatal("last edit lost", err)
	}
}
func TestNoteExternalWriterIsNeverOverwrittenOnClose(t *testing.T) {
	w, n := noteTestWindow(t)
	n.newNote()
	n.editor.SetText("local")
	n.save()
	pumpShell(t, w, func() bool { return !n.saving })
	external, err := n.service.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	external.Notes[0].Body = "external"
	// Advance the store directly through another service, excluding own commit
	// tracking. Local shutdown must reject this conflicting revision.
	other := application.Notebook{Store: n.service.Store, Vault: n.service.Vault}
	if _, err := other.Save(context.Background(), external); err != nil {
		t.Fatal(err)
	}
	n.editor.SetText("local later")
	if err := w.noteShutdownSnapshot()(context.Background()); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("external update overwritten", err)
	}
}
func TestNoteLimitNeverImportsOverExistingNote(t *testing.T) {
	_, n := noteTestWindow(t)
	n.newNote()
	id := n.selected
	n.editor.SetText("keep")
	for len(n.book.Notes) < domain.MaxNotes {
		n.book.Notes = append(n.book.Notes, domain.Note{ID: fmt.Sprintf("fixture-%d", len(n.book.Notes))})
	}
	n.newNote()
	if n.selected != id || n.current().Body != "keep" || len(n.book.Notes) != domain.MaxNotes {
		t.Fatal("capacity changed current note")
	}
	n.toggleDeleted()
	n.trash = true
	n.filter()
	n.selectNote(id)
	n.deleteCurrent()
	if len(n.book.Notes) != domain.MaxNotes-1 {
		t.Fatal("trash purge did not release capacity")
	}
}
