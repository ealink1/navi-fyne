package drivers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallRejectsExecutableScriptsBeforeProbe(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "script")
	raw := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(source, raw, 0700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	m := &Manager{Root: filepath.Join(root, "drivers")}
	if err := m.Install(context.Background(), "sqlite", source, hex.EncodeToString(sum[:]), "test"); err == nil {
		t.Fatal("executed a non-native driver")
	}
	entries, err := os.ReadDir(filepath.Join(m.Root, "sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("invalid driver left executable or metadata behind")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = validateArchitecture(executable); err != nil {
		t.Fatal("rejected native executable", err)
	}
}
func TestVerificationWaitCanBeCancelled(t *testing.T) {
	m := &Manager{Root: t.TempDir()}
	unlock, err := m.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = m.Verify(ctx, "sqlite"); !errors.Is(err, context.Canceled) {
		t.Fatal("uncancellable driver lock", err)
	}
}
