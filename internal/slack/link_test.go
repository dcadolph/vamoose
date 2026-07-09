package slack

import (
	"testing"
	"time"
)

// TestLinkStateStore covers issue, single-use consume, and unknown or empty tokens.
func TestLinkStateStore(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	s := newLinkStateStore(func() time.Time { return now })
	tok := s.issue("T1", "U1", "google")

	// Test 0: A fresh token consumes to its pending link exactly once.
	st, ok := s.consume(tok)
	if !ok || st.team != "T1" || st.user != "U1" || st.provider != "google" {
		t.Fatalf("consume = %+v, %v; want {T1 U1 google}, true", st, ok)
	}
	if _, ok := s.consume(tok); ok {
		t.Error("token consumed twice")
	}

	// Test 1: An unknown token is rejected.
	if _, ok := s.consume("nope"); ok {
		t.Error("unknown token accepted")
	}

	// Test 2: An empty token is rejected.
	if _, ok := s.consume(""); ok {
		t.Error("empty token accepted")
	}
}

// TestLinkStateExpiry confirms a pending link past its TTL is rejected.
func TestLinkStateExpiry(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	s := newLinkStateStore(func() time.Time { return now })
	tok := s.issue("T", "U", "google")
	now = now.Add(11 * time.Minute)
	if _, ok := s.consume(tok); ok {
		t.Error("expired token accepted")
	}
}
