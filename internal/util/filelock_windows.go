//go:build windows

package util

import (
	"os"

	"golang.org/x/sys/windows"
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
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if !wait {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, new(windows.Overlapped)); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
		_ = f.Close()
	}, nil
}
