package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/infra/state"
)

func TestShellCredentialsRestartTrustAndIsolation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := &ShellHosts{Store: store, Vault: secrets.NewLocal(root)}
	h, err := s.Save(ctx, domain.ShellHost{Name: "fixture", Host: "127.0.0.1", Port: 22, User: "tester", Password: "shell-secret", Passphrase: "key-secret", Remember: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := store.ShellHost(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(raw)
	if strings.Contains(string(encoded), "secret") || raw.Password != "" {
		t.Fatal("plaintext metadata")
	}
	s.Vault = secrets.NewLocal(root)
	loaded, err := s.Get(ctx, h.ID)
	if err != nil || loaded.Password != "shell-secret" || loaded.Passphrase != "key-secret" {
		t.Fatal("credentials lost", err)
	}
	if err := s.Trust(ctx, h.ID, h.Revision, "SHA256:fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.Trust(ctx, h.ID, h.Revision, "SHA256:other"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale trust accepted", err)
	}
	loaded, err = s.Get(ctx, h.ID)
	if err != nil || loaded.Fingerprint != "SHA256:fixture" || loaded.SecretRef != h.SecretRef {
		t.Fatal("trust replaced credentials", err)
	}
	if _, err := s.Save(ctx, h); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale edit accepted", err)
	}
	loaded.Host = "localhost"
	loaded, err = s.Save(ctx, loaded)
	if err != nil || loaded.Fingerprint != "" {
		t.Fatal("endpoint reused trust", err)
	}
	profiles, err := store.Profiles(ctx)
	if err != nil || len(profiles) != 0 {
		t.Fatal("Shell polluted SQL profiles", err)
	}
	if err := s.Delete(ctx, loaded.ID, loaded.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, loaded.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("delete failed", err)
	}
}
func TestShellSessionCredentialsAndEmptyKeyPassword(t *testing.T) {
	p := testProfiles(t)
	s := &ShellHosts{Store: p.Store, Vault: p.Vault}
	ctx := context.Background()
	h, err := s.Save(ctx, domain.ShellHost{Name: "session", Host: "localhost", Port: 22, User: "tester", Password: "transient"})
	if err != nil {
		t.Fatal(err)
	}
	s.Vault = secrets.New()
	if _, err := s.Get(ctx, h.ID); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("session credential survived restart", err)
	}
	h, err = s.Save(ctx, domain.ShellHost{Name: "key", Host: "localhost", Port: 22, User: "tester", KeyPath: "/fixture/key"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, h.ID); err != nil {
		t.Fatal("passwordless key metadata inaccessible", err)
	}
}
