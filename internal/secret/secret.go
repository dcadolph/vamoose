// Package secret seals small values for storage on a headless host, where the OS
// keychain is not available. It uses AES-256-GCM with a key supplied at runtime, so
// tokens and links are encrypted at rest rather than left in a plaintext file.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// KeyEnv is the environment variable holding the base64-encoded 32-byte key.
const KeyEnv = "VAMOOSE_SECRET_KEY"

// ErrNoKey means no encryption key is configured, so callers can fall back to the OS
// keychain or a plaintext file.
var ErrNoKey = errors.New("secret: no key")

// Box seals and opens values with a symmetric key.
type Box struct {
	// gcm is the authenticated cipher built from the key.
	gcm cipher.AEAD
}

// FromEnv builds a Box from the base64-encoded 32-byte key in KeyEnv, read through the
// given lookup. It returns ErrNoKey when the variable is unset.
func FromEnv(getenv func(string) string) (*Box, error) {
	raw := getenv(KeyEnv)
	if raw == "" {
		return nil, ErrNoKey
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("secret: decode %s: %w", KeyEnv, err)
	}
	return NewBox(key)
}

// NewBox builds a Box from a 32-byte key.
func NewBox(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{gcm: gcm}, nil
}

// Seal encrypts plaintext, returning a random nonce prepended to the ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal, failing when the key is wrong or the data has
// been tampered with.
func (b *Box) Open(sealed []byte) ([]byte, error) {
	ns := b.gcm.NonceSize()
	if len(sealed) < ns {
		return nil, errors.New("secret: ciphertext too short")
	}
	nonce, ciphertext := sealed[:ns], sealed[ns:]
	return b.gcm.Open(nil, nonce, ciphertext, nil)
}

// GenerateKey returns a new base64-encoded 32-byte key, for first-time setup.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
