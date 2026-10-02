package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
	"github.com/ealink1/navi-fyne/internal/infra/state"
	"github.com/google/uuid"
)

// ShellHosts owns SSH host credentials separately from database connections.
type ShellHosts struct {
	Store *state.Store
	Vault secrets.Vault
	mu    sync.Mutex
}

type shellCredentials struct{ Password, Passphrase string }

// List returns public host metadata.
func (s *ShellHosts) List(ctx context.Context) ([]domain.ShellHost, error) {
	return s.Store.ShellHosts(ctx)
}

// Get restores remembered credentials for an explicit connect or edit action.
func (s *ShellHosts) Get(ctx context.Context, id string) (domain.ShellHost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.Store.ShellHost(ctx, id)
	if err != nil {
		return h, err
	}
	raw, err := s.Vault.Get(h.SecretRef)
	if err != nil {
		return h, err
	}
	var c shellCredentials
	if err := json.Unmarshal(raw, &c); err != nil {
		return h, err
	}
	h.Password, h.Passphrase = c.Password, c.Passphrase
	return h, nil
}

// Save publishes metadata before retiring the old encrypted credentials.
func (s *ShellHosts) Save(ctx context.Context, h domain.ShellHost) (domain.ShellHost, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h.Name, h.Host, h.User = strings.TrimSpace(h.Name), strings.TrimSpace(h.Host), strings.TrimSpace(h.User)
	if err := h.Validate(); err != nil {
		return h, err
	}
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	var old domain.ShellHost
	if h.Revision > 0 {
		var err error
		old, err = s.Store.ShellHost(ctx, h.ID)
		if err != nil {
			return h, err
		}
		if old.Revision != h.Revision {
			return h, domain.ErrConflict
		}
		if old.Host != h.Host || old.Port != h.Port {
			h.Fingerprint = ""
		}
	}
	raw, err := json.Marshal(shellCredentials{Password: h.Password, Passphrase: h.Passphrase})
	if err != nil {
		return h, err
	}
	if h.Password == "" && h.Passphrase == "" {
		raw = nil
	}
	ref, err := s.Vault.Put(raw, h.Remember)
	if err != nil {
		return h, err
	}
	h.SecretRef = ref
	saved, err := s.Store.SaveShellHost(ctx, h)
	if err != nil {
		return h, errors.Join(err, s.Vault.Delete(ref))
	}
	if old.SecretRef != "" {
		err = s.Vault.Delete(old.SecretRef)
	}
	return saved, err
}

// Trust records a verified fingerprint without re-reading or replacing secrets.
func (s *ShellHosts) Trust(ctx context.Context, id string, revision int64, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.Store.ShellHost(ctx, id)
	if err != nil {
		return err
	}
	if h.Revision != revision || h.Fingerprint != "" || !strings.HasPrefix(fingerprint, "SHA256:") {
		return domain.ErrConflict
	}
	h.Fingerprint = fingerprint
	_, err = s.Store.SaveShellHost(ctx, h)
	return err
}

// Delete retires credentials after successful metadata removal.
func (s *ShellHosts) Delete(ctx context.Context, id string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.Store.ShellHost(ctx, id)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteShellHost(ctx, id, revision); err != nil {
		return err
	}
	return s.Vault.Delete(h.SecretRef)
}
