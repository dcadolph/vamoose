package audit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/secret"
)

// at is a fixed timestamp for deterministic records.
var at = time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

// TestFileStoreRecordAndEvents confirms events round-trip in order through a plaintext
// store. It uses a temp dir, so it can run in parallel.
func TestFileStoreRecordAndEvents(t *testing.T) {
	t.Parallel()
	s, err := NewFileStore(filepath.Join(t.TempDir(), "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Record(ctx, Event{Time: at, HoldID: "H1", Action: ActionCreated}); err != nil {
		t.Fatal(err)
	}
	if err := s.Record(ctx, Event{Time: at, HoldID: "H1", Action: ActionApproved, Actor: "boss@x.com"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Action != ActionCreated || got[1].Action != ActionApproved || got[1].Actor != "boss@x.com" {
		t.Fatalf("events = %+v, want created then approved by boss", got)
	}

	// A fresh store over the same file reads the history back.
	s2, err := NewFileStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s2.Events(); err != nil || len(again) != 2 {
		t.Fatalf("reopen events = %+v, %v; want 2", again, err)
	}
}

// TestFileStoreEmpty confirms an absent file reads as no events.
func TestFileStoreEmpty(t *testing.T) {
	t.Parallel()
	s, err := NewFileStore(filepath.Join(t.TempDir(), "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Events(); err != nil || got != nil {
		t.Fatalf("empty Events = %+v, %v; want nil, nil", got, err)
	}
}

// TestFileStoreCap confirms the store keeps only the most recent max events.
func TestFileStoreCap(t *testing.T) {
	t.Parallel()
	s := &FileStore{path: filepath.Join(t.TempDir(), "audit.json"), max: 3}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.Record(ctx, Event{Time: at, HoldID: fmt.Sprintf("H%d", i), Action: ActionCreated}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Events()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("kept %d events, want 3", len(got))
	}
	if got[0].HoldID != "H2" || got[2].HoldID != "H4" {
		t.Errorf("kept %s..%s, want H2..H4 (oldest dropped)", got[0].HoldID, got[2].HoldID)
	}
}

// TestEncryptedAuditStore confirms the history is encrypted at rest, reads back with the
// same key, and fails with the wrong key. It sets process environment, so it is not
// parallel.
func TestEncryptedAuditStore(t *testing.T) {
	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, key)
	path := filepath.Join(t.TempDir(), "audit.enc")

	s, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Record(context.Background(), Event{Time: at, HoldID: "H1", Action: ActionApproved, Actor: "boss@x.com"}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range [][]byte{[]byte("boss@x.com"), []byte("approved"), []byte("H1")} {
		if bytes.Contains(raw, leak) {
			t.Errorf("audit file leaks %q at rest", leak)
		}
	}

	// A fresh store with the same key reads it back.
	s2, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s2.Events(); err != nil || len(got) != 1 || got[0].Actor != "boss@x.com" {
		t.Errorf("reopen events = %+v, %v", got, err)
	}

	// A different key cannot open the file.
	other, err := secret.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(secret.KeyEnv, other)
	s3, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s3.Events(); err == nil {
		t.Error("a store with the wrong key should fail to open the history")
	}
}

// TestAuditStoreNoKeyIsPlaintext confirms the store stays plaintext without a key.
func TestAuditStoreNoKeyIsPlaintext(t *testing.T) {
	t.Setenv(secret.KeyEnv, "")
	path := filepath.Join(t.TempDir(), "audit.json")
	s, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Record(context.Background(), Event{Time: at, HoldID: "H1", Action: ActionCreated}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("created")) {
		t.Error("a store with no key should write readable JSON")
	}
}
