package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteFileAtomic covers writing a new file, overwriting an existing one, and the
// permission set, and confirms no temporary file is left behind.
func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Seed    string
		Content string
		Perm    os.FileMode
	}{{ // Test 0: A new file gets the content and permission.
		Content: "hello", Perm: 0o600,
	}, { // Test 1: An existing file is overwritten.
		Seed: "old and longer content", Content: "new", Perm: 0o600,
	}, { // Test 2: A more restrictive permission is honored.
		Content: "secret", Perm: 0o400,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "state.json")
			if test.Seed != "" {
				if err := os.WriteFile(path, []byte(test.Seed), 0o600); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}
			if err := WriteFileAtomic(path, []byte(test.Content), test.Perm); err != nil {
				t.Fatalf("WriteFileAtomic: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != test.Content {
				t.Errorf("content = %q, want %q", got, test.Content)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if info.Mode().Perm() != test.Perm {
				t.Errorf("perm = %v, want %v", info.Mode().Perm(), test.Perm)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("readdir: %v", err)
			}
			if len(entries) != 1 {
				t.Errorf("directory has %d entries, want 1 (a temporary file was left behind)", len(entries))
			}
		})
	}
}
