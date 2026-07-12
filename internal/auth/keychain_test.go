package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// TestKeychainStore confirms the keychain store migrates a legacy file token, then
// reads and writes through the keychain. It uses go-keyring's in-memory mock, so it
// cannot run in parallel with other keyring tests.
func TestKeychainStore(t *testing.T) {
	keyring.MockInit()

	fs := &FileStore{path: filepath.Join(t.TempDir(), "token.json")}
	legacy := Token{AccessToken: "old", RefreshToken: "r", Expiry: time.Now().Add(time.Hour)}
	if err := fs.Save(legacy); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	s := &keychainStore{user: "token-test", legacy: fs}

	// First load migrates from the legacy file.
	got, err := s.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AccessToken != "old" {
		t.Errorf("migrated token = %q, want old", got.AccessToken)
	}

	// The plaintext legacy file is removed once the token is in the keychain.
	if _, statErr := os.Stat(fs.path); !os.IsNotExist(statErr) {
		t.Errorf("legacy token file still present after migration, stat err = %v", statErr)
	}

	// Subsequent saves and loads go through the keychain.
	if err := s.Save(Token{AccessToken: "new"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err = s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.AccessToken != "new" {
		t.Errorf("keychain token = %q, want new", got.AccessToken)
	}
}
