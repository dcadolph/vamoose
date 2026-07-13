package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/secret"
)

// TestEncryptedStore confirms a token round-trips through the sealed file, the file is
// encrypted at rest, and NewStore selects the encrypted store when a key is set. It
// sets process environment, so it cannot run in parallel.
func TestEncryptedStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	// Windows resolves os.UserConfigDir from AppData, not HOME or XDG, so isolate it
	// too or the test writes sealed tokens into the machine's real config directory.
	t.Setenv("AppData", filepath.Join(dir, "appdata"))
	t.Setenv("LocalAppData", filepath.Join(dir, "localappdata"))
	t.Setenv("USERPROFILE", dir)
	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, key)

	s, err := NewEncryptedStore("google")
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := s.Load(); err != nil || tok.AccessToken != "" {
		t.Fatalf("empty load = %+v, %v; want zero token", tok, err)
	}

	want := Token{AccessToken: "ya29-secret", RefreshToken: "rt-secret", Expiry: time.Now().Add(time.Hour)}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken {
		t.Errorf("load = %+v, %v", got, err)
	}

	// The file on disk is sealed: no plaintext token or field names.
	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range [][]byte{[]byte("ya29-secret"), []byte("rt-secret"), []byte("access_token")} {
		if bytes.Contains(raw, leak) {
			t.Errorf("token file leaks %q", leak)
		}
	}

	// NewStore prefers the encrypted store when a key is set.
	st, err := NewStore("google")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.(*EncryptedStore); !ok {
		t.Errorf("NewStore = %T, want *EncryptedStore when a key is set", st)
	}
}
