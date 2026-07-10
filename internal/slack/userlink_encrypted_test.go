package slack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/secret"
)

// TestEncryptedUserLinkStore confirms links round-trip, the file is encrypted at rest,
// and a fresh store with the same key reads it back after a restart. It sets process
// environment, so it cannot run in parallel.
func TestEncryptedUserLinkStore(t *testing.T) {
	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, key)
	path := filepath.Join(t.TempDir(), "links.enc")

	s, err := NewUserLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	link := UserLink{Provider: "icloud", ICloudUser: "me@icloud.com", ICloudAppPassword: "abcd-efgh"}
	if err := s.SaveLink("T1", "U1", link); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLink("T1", "U1"); err != nil || got.ICloudAppPassword != "abcd-efgh" {
		t.Errorf("get = %+v, %v", got, err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range [][]byte{[]byte("abcd-efgh"), []byte("me@icloud.com"), []byte("icloud_app_password")} {
		if bytes.Contains(raw, leak) {
			t.Errorf("link file leaks %q", leak)
		}
	}

	// A fresh store with the same key reads it back, as it would after a restart.
	s2, err := NewUserLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := s2.GetLink("T1", "U1"); err != nil || l.ICloudUser != "me@icloud.com" {
		t.Errorf("reopen get = %+v, %v", l, err)
	}
}

// TestUserLinkStoreNoKeyIsPlaintext confirms the store stays plaintext without a key.
func TestUserLinkStoreNoKeyIsPlaintext(t *testing.T) {
	t.Setenv(secret.KeyEnv, "")
	path := filepath.Join(t.TempDir(), "links.json")
	s, err := NewUserLinkStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveLink("T1", "U1", UserLink{Provider: "google", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("google")) {
		t.Error("a store with no key should write readable JSON")
	}
}
