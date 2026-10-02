// Package instance prevents concurrent applications from sharing a workspace.
package instance

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrBusy = errors.New("this workspace is already open in another Navi Fyne instance")

type Lock struct{ file *os.File }

func Acquire(root string) (*Lock, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("workspace root must be absolute")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, ".instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = tryLock(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := errors.Join(unlock(l.file), l.file.Close())
	l.file = nil
	return err
}
