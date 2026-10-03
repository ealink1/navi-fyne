package update

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ealink1/super-link/internal/infra/state"
	"github.com/ealink1/super-link/internal/upstream/appdata"
)

func backupState(r Request) error {
	store, err := state.Open(filepath.Join(r.DataRoot, "state.sqlite"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return errors.Join(store.BackupTo(ctx, r.StateBackup), store.Close())
}

// Called only after the failed new process has exited and the workspace lock
// is held. Vault contents are not changed during startup or schema migrations.
func restoreState(r Request) error {
	target := filepath.Join(r.DataRoot, "state.sqlite")
	temporary, err := os.CreateTemp(r.DataRoot, ".restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	input, err := os.Open(r.StateBackup)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	_, copyErr := io.Copy(temporary, input)
	err = errors.Join(copyErr, input.Close(), temporary.Sync(), temporary.Close())
	if err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err = os.Remove(target + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return appdata.AtomicReplaceFile(temporary.Name(), target)
}
