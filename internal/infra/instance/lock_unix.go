//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package instance

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryLock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	return err
}
func unlock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_UN) }
