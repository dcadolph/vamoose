package util

import (
	"path/filepath"
	"testing"
)

// TestLockFile confirms a held lock blocks a second non-blocking acquire, on any
// platform, and that release makes the lock available again. Each acquire opens its own
// descriptor, so this exercises the same contention a second process would.
func TestLockFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.lock")

	release1, err := LockFile(path, false)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if _, err := LockFile(path, false); err == nil {
		t.Fatal("second non-blocking lock should fail while the first is held")
	}
	release1()

	release2, err := LockFile(path, false)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	release2()
}

// TestLockFileBlocking confirms the blocking mode acquires once the holder releases.
func TestLockFileBlocking(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.lock")

	release1, err := LockFile(path, false)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	acquired := make(chan struct{})
	go func() {
		release2, lerr := LockFile(path, true)
		if lerr != nil {
			t.Errorf("blocking lock: %v", lerr)
			close(acquired)
			return
		}
		release2()
		close(acquired)
	}()
	release1()
	<-acquired
}
