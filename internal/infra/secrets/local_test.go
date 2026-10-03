package secrets

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func putFixture(t *testing.T, v *System) string {
	t.Helper()
	ref, err := v.Put([]byte(`{"password":"fixture-password","ssh":{"password":"fixture-ssh"}}`), true)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestLocalVaultSurvivesNewProcess(t *testing.T) {
	// Run the same test executable as a separate process to exclude memory cache
	// from the successful read; only a temporary workspace and fake secret are used.
	if os.Getenv("SUPERLINK_VAULT_TEST_CHILD") == "1" {
		args := os.Args
		v := NewLocal(args[len(args)-2])
		data, err := v.Get(args[len(args)-1])
		if err != nil || !bytes.Contains(data, []byte("fixture-password")) {
			t.Fatal("child process could not restore remembered credentials", err)
		}
		return
	}
	root := t.TempDir()
	v := NewLocal(root)
	ref := putFixture(t, v)
	command := exec.Command(os.Args[0], "-test.run=^TestLocalVaultSurvivesNewProcess$", "--", root, ref)
	command.Env = append(os.Environ(), "SUPERLINK_VAULT_TEST_CHILD=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("isolated process failed: %v\n%s", err, output)
	}
	dir := filepath.Join(root, "credentials")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatal("unexpected credential files", err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil || bytes.Contains(data, []byte("fixture-password")) || bytes.Contains(data, []byte("fixture-ssh")) {
			t.Fatal("plaintext credential reached disk", err)
		}
		info, err := os.Stat(path)
		if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
			t.Fatal("credential file permissions changed", err)
		}
	}
	if err = v.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err = NewLocal(root).Get(ref); !errors.Is(err, ErrMissing) {
		t.Fatal("deleted credential still readable", err)
	}
}

func TestSessionOnlyNeverWritesOrSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	v := NewLocal(root)
	ref, err := v.Put([]byte(`{"password":"fixture"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("session credentials created disk files")
	}
	if _, err = NewLocal(root).Get(ref); !errors.Is(err, ErrMissing) {
		t.Fatal("session credential survived restart", err)
	}
	if ref, err = v.Put([]byte("{}"), true); err != nil || ref != "" {
		t.Fatal("empty credential created storage", err)
	}
}

func TestLocalVaultRejectsTamperedOrSwappedFiles(t *testing.T) {
	for _, change := range []string{"tamper", "swap", "truncate", "header"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			v := NewLocal(root)
			ref := putFixture(t, v)
			name, _ := localName(ref)
			path := filepath.Join(root, "credentials", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "tamper":
				data[len(data)-1] ^= 1
			case "swap":
				second := putFixture(t, v)
				secondName, _ := localName(second)
				data, err = os.ReadFile(filepath.Join(root, "credentials", secondName))
			case "truncate":
				data = data[:len(localHeader)]
			case "header":
				data[0] ^= 1
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = v.Get(ref); !errors.Is(err, ErrCorrupt) {
				t.Fatal("modified credential accepted", err)
			}
		})
	}
}

func TestLocalVaultDoesNotReplaceMissingOrDamagedKey(t *testing.T) {
	for _, change := range []string{"remove", "short", "wrong"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			v := NewLocal(root)
			ref := putFixture(t, v)
			key := filepath.Join(root, "credentials", keyName)
			var err error
			switch change {
			case "remove":
				err = os.Remove(key)
			case "short":
				err = os.WriteFile(key, []byte("short"), 0600)
			case "wrong":
				err = os.WriteFile(key, bytes.Repeat([]byte{0}, 32), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = NewLocal(root).Get(ref); !errors.Is(err, ErrCorrupt) {
				t.Fatal("damaged key accepted", err)
			}
			if change != "wrong" {
				if _, err = NewLocal(root).Put([]byte(`{"password":"replacement"}`), true); !errors.Is(err, ErrCorrupt) {
					t.Fatal("lost key silently regenerated", err)
				}
			}
			if change == "remove" {
				if _, err = os.Stat(key); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("key was replaced after loss")
				}
			}
		})
	}
}

func TestLocalVaultValidatesReferencesAndLegacyDoesNotPrompt(t *testing.T) {
	v := NewLocal(t.TempDir())
	for _, ref := range []string{"local:v1:../../outside", "local:v1:", "local:v2:bad", "garbage"} {
		if _, err := v.Get(ref); !errors.Is(err, ErrMissing) {
			t.Fatal("invalid reference accepted", err)
		}
		if err := v.Delete(ref); !errors.Is(err, ErrMissing) {
			t.Fatal("invalid deletion accepted", err)
		}
	}
	legacy := "7f3a13dd-4e32-4927-a1ce-ab46a7f09043"
	if _, err := v.Get(legacy); !errors.Is(err, ErrLegacy) {
		t.Fatal("old keychain reference did not request re-entry", err)
	}
	if err := v.Delete(legacy); err != nil {
		t.Fatal("legacy keychain deletion attempted", err)
	}
	if ref, err := New().Put([]byte(`{"password":"fixture"}`), true); !errors.Is(err, ErrUnavailable) || ref != "" {
		t.Fatal("unconfigured vault silently persisted", err)
	}
	if ref, err := v.Put(bytes.Repeat([]byte("x"), maxBundleSize+1), true); err == nil || ref != "" {
		t.Fatal("unbounded credential accepted")
	}
	if _, err := os.Stat(filepath.Join(v.root, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid operations wrote files")
	}
}
