package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestLocalVaultRejectsUnsafeFiles(t *testing.T) {
	for _, change := range []string{"key-link", "cipher-link", "directory-link", "key-public", "cipher-public", "directory-public", "oversized"} {
		t.Run(change, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Unix permissions and unprivileged symlink tests")
			}
			root := t.TempDir()
			v := NewLocal(root)
			ref := putFixture(t, v)
			dir := filepath.Join(root, "credentials")
			name, _ := localName(ref)
			target := filepath.Join(dir, name)
			if change == "key-link" || change == "key-public" {
				target = filepath.Join(dir, keyName)
			}
			if change == "directory-link" || change == "directory-public" {
				target = dir
			}
			var err error
			switch change {
			case "key-public", "cipher-public", "directory-public":
				err = os.Chmod(target, 0755)
			case "oversized":
				err = os.Truncate(target, maxBundleSize+65)
			default:
				moved := target + ".moved"
				if err = os.Rename(target, moved); err == nil {
					err = os.Symlink(moved, target)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = NewLocal(root).Get(ref); err == nil {
				t.Fatal("unsafe credential storage accepted")
			}
		})
	}
}

func TestLocalVaultConcurrentCreatorsShareOneCompleteKey(t *testing.T) {
	root := t.TempDir()
	const count = 16
	var wait sync.WaitGroup
	refs := make([]string, count)
	errs := make([]error, count)
	for i := range count {
		wait.Go(func() {
			refs[i], errs[i] = NewLocal(root).Put([]byte(`{"password":"concurrent-fixture"}`), true)
		})
	}
	wait.Wait()
	for i, ref := range refs {
		if errs[i] != nil {
			t.Fatal("concurrent key creation failed", errs[i])
		}
		if _, err := NewLocal(root).Get(ref); err != nil {
			t.Fatal("concurrent creator used a different or partial key", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "credentials"))
	if err != nil || len(entries) != count+1 {
		t.Fatal("left temporary files or lost stored credentials", err)
	}
}

func TestPrivatePublicationDoesNotOverwriteExistingKey(t *testing.T) {
	dir := t.TempDir()
	if err := writePrivate(dir, keyName, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writePrivate(dir, keyName, []byte("second")); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing key overwritten", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, keyName))
	if err != nil || string(data) != "first" {
		t.Fatal("original key changed", err)
	}
}
