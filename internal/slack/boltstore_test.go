package slack

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/dcadolph/vamoose/internal/boltstore"
)

// openBolt opens a temp bbolt database closed on cleanup.
func openBolt(t *testing.T) *boltstore.DB {
	t.Helper()
	db, err := boltstore.Open(filepath.Join(t.TempDir(), "vamoose.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestBoltTokenStore confirms bot tokens save and load, and an uninstalled workspace
// reports an error.
func TestBoltTokenStore(t *testing.T) {
	t.Parallel()
	s := NewBoltTokenStore(openBolt(t))
	if _, err := s.Get("T1"); err == nil {
		t.Error("an uninstalled workspace should error")
	}
	if err := s.Save("T1", "xoxb-1"); err != nil {
		t.Fatal(err)
	}
	if tok, err := s.Get("T1"); err != nil || tok != "xoxb-1" {
		t.Errorf("get = %q, %v; want xoxb-1", tok, err)
	}
}

// TestBoltUserLinkStore exercises the link store: save, get, list, delete, and the
// not-linked error.
func TestBoltUserLinkStore(t *testing.T) {
	t.Parallel()
	s := NewBoltUserLinkStore(openBolt(t))
	if _, err := s.GetLink("T1", "U1"); !errors.Is(err, ErrNotLinked) {
		t.Errorf("absent link err = %v, want ErrNotLinked", err)
	}
	if err := s.SaveLink("T1", "U1", UserLink{Provider: "google", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLink("T1", "U1"); err != nil || got.RefreshToken != "rt" {
		t.Errorf("get = %+v, %v; want refresh token rt", got, err)
	}
	if err := s.SaveLink("T1", "U2", UserLink{Provider: "icloud"}); err != nil {
		t.Fatal(err)
	}
	if ids, err := s.List(); err != nil || len(ids) != 2 {
		t.Fatalf("list = %v, %v; want 2 links", ids, err)
	}
	if err := s.DeleteLink("T1", "U1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetLink("T1", "U1"); !errors.Is(err, ErrNotLinked) {
		t.Error("deleted link still present")
	}
}
