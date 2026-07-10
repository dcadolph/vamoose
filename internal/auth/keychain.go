package auth

import (
	"encoding/json"
	"errors"

	"github.com/zalando/go-keyring"

	"github.com/dcadolph/vamoose/internal/secret"
)

// keychainService is the service name tokens are stored under in the OS keychain.
const keychainService = "vamoose"

// NewStore returns the best available token store for the provider: an encrypted file
// when VAMOOSE_SECRET_KEY is set (for a headless host), otherwise the OS keychain when
// it is reachable, otherwise a plaintext file under the config directory. It never
// fails over to leave tokens unstored, so auth keeps working everywhere.
func NewStore(name string) (TokenStore, error) {
	if enc, err := NewEncryptedStore(name); err == nil {
		return enc, nil
	} else if !errors.Is(err, secret.ErrNoKey) {
		return nil, err
	}
	fs, err := NewFileStore(name)
	if err != nil {
		return nil, err
	}
	if !keychainAvailable() {
		return fs, nil
	}
	user := "token"
	if name != "" {
		user = "token-" + name
	}
	return &keychainStore{user: user, legacy: fs}, nil
}

// keychainStore stores the token in the OS keychain. On first load it migrates a
// token from the legacy file store, so existing sign-ins carry over.
type keychainStore struct {
	// user is the keychain account key, namespaced per provider.
	user string
	// legacy is the file store read once to migrate an existing token.
	legacy *FileStore
}

// Load returns the token from the keychain, migrating from the legacy file when the
// keychain has none. A missing token yields a zero Token and no error.
func (s *keychainStore) Load() (Token, error) {
	secret, err := keyring.Get(keychainService, s.user)
	if err == nil {
		var t Token
		if jerr := json.Unmarshal([]byte(secret), &t); jerr != nil {
			return Token{}, jerr
		}
		return t, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return Token{}, err
	}
	if s.legacy != nil {
		if t, ferr := s.legacy.Load(); ferr == nil && t.AccessToken != "" {
			_ = s.Save(t)
			return t, nil
		}
	}
	return Token{}, nil
}

// Save writes the token to the keychain.
func (s *keychainStore) Save(t Token) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return keyring.Set(keychainService, s.user, string(b))
}

// keychainAvailable reports whether the OS keychain can be written and read, so the
// caller can fall back to file storage where it cannot.
func keychainAvailable() bool {
	const probe = "vamoose-probe"
	if err := keyring.Set(keychainService, probe, "1"); err != nil {
		return false
	}
	_, err := keyring.Get(keychainService, probe)
	_ = keyring.Delete(keychainService, probe)
	return err == nil
}
