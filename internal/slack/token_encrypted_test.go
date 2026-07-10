package slack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/secret"
)

// TestEncryptedTokenStore confirms bot tokens round-trip, the file is encrypted at rest,
// a fresh store with the same key reads it back, and the wrong key fails to open. It sets
// process environment, so it cannot run in parallel.
func TestEncryptedTokenStore(t *testing.T) {
	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, key)
	path := filepath.Join(t.TempDir(), "slack-tokens.enc")

	s, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("T1", "xoxb-secret-bot-token"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("T1"); err != nil || got != "xoxb-secret-bot-token" {
		t.Errorf("get = %q, %v", got, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("xoxb-secret-bot-token")) {
		t.Error("token file leaks the bot token in plaintext")
	}

	// A fresh store with the same key reads it back, as it would after a restart.
	s2, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s2.Get("T1"); err != nil || got != "xoxb-secret-bot-token" {
		t.Errorf("reopen get = %q, %v", got, err)
	}

	// A different key cannot open the file.
	other, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, other)
	s3, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s3.Get("T1"); err == nil {
		t.Error("a store with the wrong key should fail to open the file")
	}
}

// TestTokenStoreNoKeyIsPlaintext confirms the store stays plaintext without a key.
func TestTokenStoreNoKeyIsPlaintext(t *testing.T) {
	t.Setenv(secret.KeyEnv, "")
	path := filepath.Join(t.TempDir(), "slack-tokens.json")
	s, err := NewTokenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save("T1", "xoxb-plain"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("xoxb-plain")) {
		t.Error("a store with no key should write readable JSON")
	}
}
