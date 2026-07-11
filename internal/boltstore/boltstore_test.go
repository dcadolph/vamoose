package boltstore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/secret"
)

// openTest opens a plaintext DB in a temp dir and closes it on cleanup.
func openTest(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestBoltPutGetDeleteList exercises the key-value operations.
func TestBoltPutGetDeleteList(t *testing.T) {
	t.Parallel()
	db := openTest(t)
	if err := db.Put("b", "k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := db.Get("b", "k"); err != nil || !ok || string(v) != "v" {
		t.Fatalf("get = %q, %v, %v; want v", v, ok, err)
	}
	if _, ok, _ := db.Get("b", "missing"); ok {
		t.Error("a missing key should not be found")
	}
	if _, ok, _ := db.Get("absent-bucket", "k"); ok {
		t.Error("a missing bucket should not be found")
	}
	if err := db.Put("b", "k2", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	m, err := db.List("b")
	if err != nil || len(m) != 2 || string(m["k2"]) != "v2" {
		t.Fatalf("list = %v, %v; want 2 entries", m, err)
	}
	if err := db.Delete("b", "k"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.Get("b", "k"); ok {
		t.Error("deleted key still present")
	}
}

// TestBoltPersists confirms data survives a close and reopen.
func TestBoltPersists(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Put("b", "k", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if v, ok, _ := db2.Get("b", "k"); !ok || string(v) != "v" {
		t.Errorf("reopen get = %q, %v; want v", v, ok)
	}
}

// TestBoltEncryption confirms values are sealed at rest, read back with the same key, and
// fail with the wrong key. It sets process environment, so it is not parallel.
func TestBoltEncryption(t *testing.T) {
	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, key)
	path := filepath.Join(t.TempDir(), "enc.db")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Put("secrets", "tok", []byte("xoxb-super-secret")); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("xoxb-super-secret")) {
		t.Error("value leaked in plaintext in the db file")
	}

	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := db2.Get("secrets", "tok"); !ok || string(v) != "xoxb-super-secret" {
		t.Errorf("reopen with the key = %q, %v", v, ok)
	}
	_ = db2.Close()

	other, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, other)
	db3, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db3.Close()
	if _, _, err := db3.Get("secrets", "tok"); err == nil {
		t.Error("the wrong key should fail to decrypt the value")
	}
}

// TestBoltManyKeys confirms the store handles many keys, for multi-tenant scale.
func TestBoltManyKeys(t *testing.T) {
	t.Parallel()
	db := openTest(t)
	const n = 300
	for i := 0; i < n; i++ {
		if err := db.Put("tenants", fmt.Sprintf("t%d", i), []byte(fmt.Sprintf("v%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	m, err := db.List("tenants")
	if err != nil || len(m) != n {
		t.Fatalf("list = %d entries, %v; want %d", len(m), err, n)
	}
	if v, ok, _ := db.Get("tenants", "t150"); !ok || string(v) != "v150" {
		t.Errorf("get t150 = %q, %v", v, ok)
	}
}
