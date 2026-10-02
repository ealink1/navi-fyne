package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// BackupTo uses SQLite's consistent snapshot, including committed WAL pages.
// It deliberately refuses to overwrite an existing backup.
func (s *Store) BackupTo(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("backup path must be absolute")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup destination already exists")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "VACUUM INTO '"+strings.ReplaceAll(path, "'", "''")+"'")
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return os.Chmod(path, 0600)
}
