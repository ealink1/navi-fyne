package application

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
	"github.com/ealink1/navi-fyne/internal/infra/state"
)

// Notebook owns encrypted persistence and serializes saves for one editor.
type Notebook struct {
	Store     *state.Store
	Vault     secrets.Vault
	mu        sync.Mutex
	committed int64
}

// NoteSave distinguishes a committed snapshot from nonfatal ciphertext cleanup.
type NoteSave struct {
	Book         domain.Notebook
	CleanupError error
}

// Load restores an entire bounded library for local search and editing.
func (s *Notebook) Load(ctx context.Context) (domain.Notebook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, err := s.Store.NotebookReference(ctx)
	if err != nil {
		return domain.Notebook{}, err
	}
	book := domain.Notebook{Revision: ref.Revision}
	if ref.Ref == "" {
		return book, nil
	}
	if err := ctx.Err(); err != nil {
		return book, err
	}
	raw, err := s.Vault.Get(ref.Ref)
	if err != nil {
		return book, errors.New("笔记无法解密，请恢复完整工作区备份")
	}
	defer clear(raw)
	if len(raw) > domain.MaxNotebookBytes {
		return book, errors.New("笔记本超过读取限制")
	}
	if err = json.Unmarshal(raw, &book); err != nil {
		return book, errors.New("笔记内容损坏，无法加载")
	}
	book.Revision = ref.Revision
	return book, book.Validate()
}

// Save publishes ciphertext before retiring the previous complete snapshot.
func (s *Notebook) Save(ctx context.Context, book domain.Notebook) (NoteSave, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return NoteSave{}, err
	}
	if err := book.Validate(); err != nil {
		return NoteSave{}, err
	}
	old, err := s.Store.NotebookReference(ctx)
	if err != nil {
		return NoteSave{}, err
	}
	if old.Revision != book.Revision {
		return NoteSave{}, domain.ErrConflict
	}
	raw, err := json.Marshal(book)
	if err != nil {
		return NoteSave{}, err
	}
	defer clear(raw)
	ref, err := s.Vault.Put(raw, true)
	if err != nil {
		return NoteSave{}, errors.New("笔记加密保存失败，请检查工作区权限和可用空间")
	}
	revision, err := s.Store.PublishNotebook(ctx, book.Revision, ref)
	if err != nil {
		return NoteSave{}, errors.Join(err, s.Vault.Delete(ref))
	}
	book.Revision = revision
	s.committed = revision
	return NoteSave{Book: book, CleanupError: s.Vault.Delete(old.Ref)}, nil
}

// CommittedRevision covers a successful save whose UI callback was cancelled
// during shutdown. It never rebases over changes made by another service.
func (s *Notebook) CommittedRevision() int64 { s.mu.Lock(); defer s.mu.Unlock(); return s.committed }
