package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ealink1/super-link/internal/infra/chat"
	"github.com/ealink1/super-link/internal/infra/secrets"
)

func TestAISettingsEncryptedRestartAndReplacement(t *testing.T) {
	notes, root := notebookFixture(t)
	s := &AISettings{Store: notes.Store, Vault: notes.Vault}
	ctx := context.Background()
	config := chat.Config{BaseURL: "https://example.com/v1", Model: "fixture-model", APIKey: "ai-private-fixture-key", Stream: true}
	initial, err := s.Load(ctx)
	if err != nil || initial.Model != "" {
		t.Fatal(initial, err)
	}
	result, err := s.Save(ctx, config)
	if err != nil || result.CleanupError != nil {
		t.Fatal(err, result.CleanupError)
	}
	ref, err := s.Store.Setting(ctx, aiSettingKey)
	if err != nil || strings.Contains(ref, config.APIKey) || !strings.HasPrefix(ref, "local:") {
		t.Fatal("invalid public reference", err)
	}
	restarted := &AISettings{Store: s.Store, Vault: secrets.NewLocal(root)}
	restored, err := restarted.Load(ctx)
	if err != nil || restored != config {
		t.Fatal("restart lost settings", err)
	}
	config.APIKey = "replacement-private-key"
	if _, err := restarted.Save(ctx, config); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Vault.Get(ref); !errors.Is(err, secrets.ErrMissing) {
		t.Fatal("old ciphertext retained", err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "private-fixture-key") || strings.Contains(string(raw), config.APIKey) {
			t.Errorf("plaintext key in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

type cancellingAIVault struct {
	secrets.Vault
	cancel context.CancelFunc
}

func (v cancellingAIVault) Put(raw []byte, persistent bool) (string, error) {
	ref, err := v.Vault.Put(raw, persistent)
	v.cancel()
	return ref, err
}

func TestAISettingsFailedPublishPreservesCommittedConfig(t *testing.T) {
	notes, root := notebookFixture(t)
	s := &AISettings{Store: notes.Store, Vault: notes.Vault}
	original := chat.Config{BaseURL: "https://example.com/v1", Model: "original", APIKey: "fixture-key"}
	if _, err := s.Save(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Vault = cancellingAIVault{Vault: notes.Vault, cancel: cancel}
	replacement := original
	replacement.Model = "replacement"
	if _, err := s.Save(ctx, replacement); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.Vault = secrets.NewLocal(root)
	loaded, err := s.Load(context.Background())
	if err != nil || loaded != original {
		t.Fatal("failed save destroyed committed config", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "credentials", "*.cred"))
	if err != nil || len(files) != 1 {
		t.Fatal("failed publish orphaned ciphertext", files, err)
	}
}
