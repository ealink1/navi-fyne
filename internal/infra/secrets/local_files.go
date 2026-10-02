package secrets

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// readPrivate rejects symlinks, replaced files, unsafe permissions and oversized
// data before reading. Error messages exposed by the vault omit file contents.
func readPrivate(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !privatePermissions(info) || info.Size() > limit {
		return nil, ErrCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(ErrCorrupt, file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	err = errors.Join(readErr, file.Close())
	if err != nil || int64(len(data)) > limit {
		clear(data)
		return nil, ErrCorrupt
	}
	return data, nil
}

// Publish a complete, synced file without replacing any existing key/ciphertext.
// A temporary hard link prevents interrupted key creation from leaving a partial
// master key and lets a concurrent creator reuse the winning key safely.
func writePrivate(dir, name string, data []byte) (err error) {
	file, err := os.CreateTemp(dir, ".credential-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { err = errors.Join(err, os.Remove(temporary)) }()
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	target := filepath.Join(dir, name)
	if err = os.Link(temporary, target); err != nil {
		return err
	}
	if err = syncDirectory(dir); err != nil {
		return errors.Join(err, os.Remove(target))
	}
	return nil
}
