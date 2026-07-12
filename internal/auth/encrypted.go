package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/dcadolph/vamoose/internal/secret"
	"github.com/dcadolph/vamoose/internal/util"
)

// EncryptedStore stores the token as an AES-256-GCM sealed file, for a headless host
// where the OS keychain is unavailable. The key comes from VAMOOSE_SECRET_KEY, so
// tokens are encrypted at rest rather than left in a plaintext file.
type EncryptedStore struct {
	// path is the sealed token file location.
	path string
	// box seals and opens the token bytes.
	box *secret.Box
}

// NewEncryptedStore returns an encrypted token store for the provider name, or
// secret.ErrNoKey when no key is configured so the caller can fall back.
func NewEncryptedStore(name string) (*EncryptedStore, error) {
	box, err := secret.FromEnv(os.Getenv)
	if err != nil {
		return nil, err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	file := "token.enc"
	if name != "" {
		file = "token-" + name + ".enc"
	}
	return &EncryptedStore{path: filepath.Join(dir, "vamoose", file), box: box}, nil
}

// Load reads and decrypts the token file. A missing file yields a zero Token.
func (s *EncryptedStore) Load() (Token, error) {
	sealed, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Token{}, nil
	}
	if err != nil {
		return Token{}, err
	}
	plain, err := s.box.Open(sealed)
	if err != nil {
		return Token{}, err
	}
	var t Token
	if err := json.Unmarshal(plain, &t); err != nil {
		return Token{}, err
	}
	return t, nil
}

// Save encrypts and writes the token file, creating parent directories as needed.
func (s *EncryptedStore) Save(t Token) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	sealed, err := s.box.Seal(b)
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(s.path, sealed, 0o600)
}
