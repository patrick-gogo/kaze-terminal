//go:build !windows

package sessionstate

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func lockFile(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return ErrOwned
	}
	return err
}
func unlockFile(f *os.File)             { unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func replaceFile(src, dst string) error { return os.Rename(src, dst) }

// syncFolder makes a rename in dir durable across a power loss.
func syncFolder(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
