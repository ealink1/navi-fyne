//go:build !windows

package secrets

import (
	"errors"
	"os"
)

func privatePermissions(info os.FileInfo) bool {
	return info.Mode().Perm()&0077 == 0
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}
