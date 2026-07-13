//go:build unix

package util

import (
	"os"
	"syscall"
)

// LockFile takes an exclusive lock on the file at path, creating it if absent. When wait
// is false and another process holds the lock, it returns an error immediately rather
// than blocking. The returned release unlocks and closes the file without removing it.
// The parent directory must exist.
func LockFile(path string, wait bool) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_EX
	if !wait {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
