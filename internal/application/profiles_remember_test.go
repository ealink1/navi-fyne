package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/domain"
	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/infra/state"
	"github.com/ealink1/super-link/internal/upstream/connection"
)

func rememberedProfiles(t *testing.T) (*Profiles, string) {
	t.Helper()
	root := t.TempDir()
	store, err := state.Open(filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Profiles{Store: store, Vault: secrets.NewLocal(root)}, root
}

func TestRememberedProfileRestoresReplacesAndForgets(t *testing.T) {
	ctx := context.Background()
	p, root := rememberedProfiles(t)
	profile, err := p.Save(ctx, domain.Profile{Name: "fixture", Config: connection.ConnectionConfig{Type: "mysql", Password: "first-fixture", SavePassword: true, SSH: connection.SSHConfig{Password: "ssh-fixture"}}})
	if err != nil {
		t.Fatal(err)
	}
	p.Vault = secrets.NewLocal(root)
	loaded, err := p.Get(ctx, profile.ID)
	if err != nil || loaded.Config.Password != "first-fixture" || loaded.Config.SSH.Password != "ssh-fixture" {
		t.Fatal("saved profile lost credentials on restart", err)
	}
	oldRef := loaded.SecretRef
	loaded.Config.Password = "replacement-fixture"
	loaded, err = p.Save(ctx, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Vault.Get(oldRef); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("replaced bundle remained readable", err)
	}
	p.Vault = secrets.NewLocal(root)
	loaded, err = p.Get(ctx, profile.ID)
	if err != nil || loaded.Config.Password != "replacement-fixture" {
		t.Fatal("replacement lost on restart", err)
	}
	oldRef = loaded.SecretRef
	loaded.Config.SavePassword = false
	loaded, err = p.Save(ctx, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Vault.Get(oldRef); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("unchecking remember retained persisted secret", err)
	}
	if _, err = p.Get(ctx, profile.ID); err != nil {
		t.Fatal("forget interrupted current session", err)
	}
	p.Vault = secrets.NewLocal(root)
	if _, err = p.Get(ctx, profile.ID); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("forgotten credential survived restart", err)
	}
	stored, err := p.Store.Profile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stored)
	if err != nil || strings.Contains(string(raw), "replacement-fixture") || strings.Contains(string(raw), "ssh-fixture") {
		t.Fatal("plaintext credential reached public metadata", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "credentials"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "master.key" {
		t.Fatal("forgotten encrypted bundle remained on disk", err)
	}
}

func TestRememberedProfileFailedCommitPreservesPreviousSecret(t *testing.T) {
	ctx := context.Background()
	p, root := rememberedProfiles(t)
	loaded, err := p.Save(ctx, domain.Profile{Name: "fixture", Config: connection.ConnectionConfig{Type: "mysql", Password: "fixture", SavePassword: true}})
	if err != nil {
		t.Fatal(err)
	}
	// A duplicate insert fails after the replacement bundle has been published;
	// the old secret must survive and the new orphan must be removed.
	stale := loaded
	stale.Revision = 0
	stale.Config.Password = "replacement"
	if _, err = p.Save(ctx, stale); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("conflicting insert accepted", err)
	}
	current, err := p.Get(ctx, loaded.ID)
	if err != nil || current.Config.Password != "fixture" {
		t.Fatal("failed commit destroyed old credential", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "credentials"))
	if err != nil || len(entries) != 2 {
		t.Fatal("failed commit leaked replacement secret", err)
	}
	if err = p.Delete(ctx, current.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Vault.Get(current.SecretRef); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("deleted profile retained secret", err)
	}
}
