package secret

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

// testKey returns a fixed 32-byte key for tests.
func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// TestSealOpen confirms a value round-trips and the sealed form hides the plaintext.
func TestSealOpen(t *testing.T) {
	t.Parallel()
	b, err := NewBox(testKey())
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("token: xoxb-secret")
	sealed, err := b.Seal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, msg) {
		t.Error("sealed value leaks the plaintext")
	}
	got, err := b.Open(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Errorf("open = %q, want %q", got, msg)
	}
}

// TestOpenWrongKeyFails confirms the wrong key cannot open a sealed value.
func TestOpenWrongKeyFails(t *testing.T) {
	t.Parallel()
	a, _ := NewBox(testKey())
	other := make([]byte, 32)
	other[0] = 0xff
	b, _ := NewBox(other)
	sealed, _ := a.Seal([]byte("secret"))
	if _, err := b.Open(sealed); err == nil {
		t.Error("open with the wrong key should fail")
	}
}

// TestOpenTamperedFails confirms tampered or truncated data is rejected.
func TestOpenTamperedFails(t *testing.T) {
	t.Parallel()
	b, _ := NewBox(testKey())
	sealed, _ := b.Seal([]byte("secret"))
	sealed[len(sealed)-1] ^= 0xff
	if _, err := b.Open(sealed); err == nil {
		t.Error("open of tampered ciphertext should fail")
	}
	if _, err := b.Open([]byte("x")); err == nil {
		t.Error("open of too-short data should fail")
	}
}

// TestFromEnv covers the environment key: absent, malformed, and valid.
func TestFromEnv(t *testing.T) {
	t.Parallel()
	if _, err := FromEnv(func(string) string { return "" }); !errors.Is(err, ErrNoKey) {
		t.Errorf("no key err = %v, want ErrNoKey", err)
	}
	if _, err := FromEnv(func(string) string { return "!!! not base64" }); err == nil {
		t.Error("malformed base64 should error")
	}
	key := base64.StdEncoding.EncodeToString(testKey())
	b, err := FromEnv(func(k string) string {
		if k == KeyEnv {
			return key
		}
		return ""
	})
	if err != nil || b == nil {
		t.Fatalf("valid key = %v, %v", b, err)
	}
}

// TestNewBoxKeyLength confirms a non-32-byte key is rejected.
func TestNewBoxKeyLength(t *testing.T) {
	t.Parallel()
	if _, err := NewBox([]byte("short")); err == nil {
		t.Error("want an error for a short key")
	}
}

// TestGenerateKey confirms a generated key decodes to 32 bytes, is usable, and differs
// between calls.
func TestGenerateKey(t *testing.T) {
	t.Parallel()
	k1, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(k1)
	if err != nil || len(raw) != 32 {
		t.Fatalf("generated key decoded to %d bytes, %v", len(raw), err)
	}
	if _, err := NewBox(raw); err != nil {
		t.Errorf("generated key not usable: %v", err)
	}
	if k2, _ := GenerateKey(); k1 == k2 {
		t.Error("two generated keys should differ")
	}
}
