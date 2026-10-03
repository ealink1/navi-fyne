package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/infra/state"
)

func notebookFixture(t *testing.T) (*Notebook, string) {
	t.Helper()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return &Notebook{Store: store, Vault: secrets.NewLocal(root)}, root
}
func TestNotebookEncryptedRestartAndConflict(t *testing.T) {
	s, root := notebookFixture(t)
	ctx := context.Background()
	book := domain.Notebook{Notes: []domain.Note{{ID: "one", Title: "private-note-fixture-title", Body: "private-note-fixture-body"}}}
	result, err := s.Save(ctx, book)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Notebook{Store: s.Store, Vault: secrets.NewLocal(root)}
	restored, err := restarted.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Revision != 1 || restored.Notes[0].Body != book.Notes[0].Body {
		t.Fatal("restart lost content")
	}
	if _, err := restarted.Save(ctx, book); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale snapshot accepted", err)
	}
	result.Book.Notes[0].Body = "latest-fixture-body"
	if _, err := s.Save(ctx, result.Book); err != nil {
		t.Fatal(err)
	}
	restored, err = restarted.Load(ctx)
	if err != nil || restored.Notes[0].Body != "latest-fixture-body" {
		t.Fatal("replacement failed", err)
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "private-note-fixture") || strings.Contains(string(data), "latest-fixture-body") {
			t.Error("plaintext persisted", filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type notebookVault struct {
	secrets.Vault
	afterPut   func()
	failDelete bool
	deleted    []string
}

func (v *notebookVault) Put(raw []byte, persist bool) (string, error) {
	ref, err := v.Vault.Put(raw, persist)
	if v.afterPut != nil {
		v.afterPut()
	}
	return ref, err
}
func (v *notebookVault) Delete(ref string) error {
	v.deleted = append(v.deleted, ref)
	if v.failDelete && ref != "" {
		return errors.New("cleanup fixture")
	}
	return v.Vault.Delete(ref)
}
func TestNotebookPublicationRollbackAndCleanupReceipt(t *testing.T) {
	s, _ := notebookFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	v := &notebookVault{Vault: s.Vault, afterPut: cancel}
	s.Vault = v
	book := domain.Notebook{Notes: []domain.Note{{ID: "one", Body: "fixture"}}}
	if _, err := s.Save(ctx, book); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(v.deleted) != 1 || v.deleted[0] == "" {
		t.Fatal("unpublished ciphertext retained")
	}
	ref, err := s.Store.NotebookReference(context.Background())
	if err != nil || ref.Revision != 0 {
		t.Fatal("cancelled snapshot published", err)
	}
	v.afterPut = nil
	first, err := s.Save(context.Background(), book)
	if err != nil {
		t.Fatal(err)
	}
	v.failDelete = true
	result, err := s.Save(context.Background(), first.Book)
	if err != nil || result.CleanupError == nil || result.Book.Revision != 2 {
		t.Fatal("cleanup confused with save failure", err)
	}
	loaded, err := s.Load(context.Background())
	if err != nil || loaded.Revision != 2 {
		t.Fatal("committed snapshot inaccessible", err)
	}
}
func TestNotebookInvalidSnapshotPreservesOld(t *testing.T) {
	s, _ := notebookFixture(t)
	ctx := context.Background()
	good, err := s.Save(ctx, domain.Notebook{Notes: []domain.Note{{ID: "one", Body: "kept"}}})
	if err != nil {
		t.Fatal(err)
	}
	bad := good.Book.Clone()
	bad.Notes[0].Body = strings.Repeat("x", domain.MaxNoteBody+1)
	if _, err := s.Save(ctx, bad); err == nil {
		t.Fatal("unbounded body accepted")
	}
	loaded, err := s.Load(ctx)
	if err != nil || loaded.Notes[0].Body != "kept" {
		t.Fatal("invalid edit replaced old content", err)
	}
}
