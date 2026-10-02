package application

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/navi-fyne/internal/domain"
	"github.com/ealink1/navi-fyne/internal/infra/secrets"
	"github.com/ealink1/navi-fyne/internal/infra/state"
	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

func testProfiles(t *testing.T) *Profiles {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Profiles{Store: store, Vault: secrets.New()}
}
func saveProfile(t *testing.T, p *Profiles, kind string, readOnly bool) domain.Profile {
	t.Helper()
	profile, err := p.Save(context.Background(), domain.Profile{Name: kind, ReadOnly: readOnly, Config: connection.ConnectionConfig{Type: kind, Password: "credential-value", Host: "127.0.0.1", Timeout: 2}})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
func TestProfilesPersistPublicMetadataAndHydrateCredentials(t *testing.T) {
	ctx := context.Background()
	p := testProfiles(t)
	saved := saveProfile(t, p, "mysql", true)
	stored, err := p.Store.Profile(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), "credential-value") {
		t.Fatal("credential reached SQLite metadata")
	}
	if stored.SecretRef == "" {
		t.Fatal("no credential reference")
	}
	hydrated, err := p.Get(ctx, saved.ID)
	if err != nil || hydrated.Config.Password != "credential-value" {
		t.Fatal("credentials were not hydrated")
	}
	// Failed persistence must not destroy the previous credential bundle.
	stale := hydrated
	stale.Revision--
	stale.Config.Password = "replacement"
	if _, err = p.Save(ctx, stale); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale save accepted")
	}
	if _, err = p.Vault.Get(stored.SecretRef); err != nil {
		t.Fatal("failed save deleted current secret")
	}
}

func TestFineGrainedProtectionIsPreserved(t *testing.T) {
	ctx := context.Background()
	p := testProfiles(t)
	saved, err := p.Save(ctx, domain.Profile{Name: "protected", Config: connection.ConnectionConfig{Type: "mysql", Protection: connection.ConnectionProtectionConfig{RestrictStructureEdit: true}}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := p.Get(ctx, saved.ID)
	if err != nil || !loaded.Config.Protection.RestrictStructureEdit || loaded.Config.Protection.RestrictDataEdit {
		t.Fatal("protection changed", err)
	}
}
