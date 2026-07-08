// Package auth obtains and caches Microsoft Graph tokens using the OAuth 2.0
// device authorization grant, so vamoose can call Graph on the user's behalf.
package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Token holds an OAuth access token and its refresh material.
type Token struct {
	// AccessToken is the bearer token sent to Graph.
	AccessToken string `json:"access_token"`
	// RefreshToken renews the access token without user interaction.
	RefreshToken string `json:"refresh_token"`
	// Scope is the space-separated set of granted scopes.
	Scope string `json:"scope"`
	// Expiry is when the access token stops being valid.
	Expiry time.Time `json:"expiry"`
}

// Valid reports whether the access token is present and not near expiry.
func (t Token) Valid() bool {
	return t.AccessToken != "" && time.Now().Before(t.Expiry.Add(-time.Minute))
}

// TokenStore persists and retrieves a cached Token.
type TokenStore interface {
	// Load returns the stored token, or a zero Token when none is stored.
	Load() (Token, error)
	// Save writes the token to storage.
	Save(Token) error
}

// FileStore stores the token as JSON under the user config directory.
type FileStore struct {
	// path is the token file location.
	path string
}

// NewFileStore returns a FileStore for the given provider name under the user
// config directory. An empty name uses the shared default token file.
func NewFileStore(name string) (*FileStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	file := "token.json"
	if name != "" {
		file = "token-" + name + ".json"
	}
	return &FileStore{path: filepath.Join(dir, "vamoose", file)}, nil
}

// Load reads the token file. A missing file yields a zero Token and no error.
func (s *FileStore) Load() (Token, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Token{}, nil
	}
	if err != nil {
		return Token{}, err
	}
	var t Token
	if err := json.Unmarshal(b, &t); err != nil {
		return Token{}, err
	}
	return t, nil
}

// Save writes the token file, creating parent directories as needed.
func (s *FileStore) Save(t Token) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
}
