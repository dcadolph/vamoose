// Package util holds small helpers shared across vamoose packages.
package util

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path so that a concurrent reader or a crash never
// observes a partially written file. It writes a temporary file in the same directory,
// flushes it to disk, then renames it over path, which is atomic on POSIX file systems.
// The destination ends up with the given permission. The parent directory must exist.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	// Remove the temporary file on any error path. After a successful rename it no
	// longer exists, so the removal is a harmless no-op.
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
