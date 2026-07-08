package cmd

import (
	"path/filepath"
	"testing"
)

// TestStateRoundTrip exercises hold state on disk against an isolated HOME.
// It cannot run in parallel because it sets process environment variables.
func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	if s, err := loadState(); err != nil || s.LastHold.ID != "" {
		t.Fatalf("loadState on empty = %+v, %v; want zero, nil", s, err)
	}
	want := holdRef{Provider: "graph", ID: "evt123"}
	if err := saveState(state{LastHold: want}); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, err := loadState()
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got.LastHold != want {
		t.Errorf("loaded = %+v, want %+v", got.LastHold, want)
	}
}

// TestResolveHold covers explicit ids, the cached hold, and the empty case.
// It cannot run in parallel because it sets process environment variables.
func TestResolveHold(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("VAMOOSE_PROVIDER", "")

	// An explicit id with no flag or env resolves to the default provider.
	ref, err := resolveHold("evt-explicit", "")
	if err != nil {
		t.Fatalf("resolveHold explicit: %v", err)
	}
	if ref.Provider != defaultProvider || ref.ID != "evt-explicit" {
		t.Errorf("explicit ref = %+v, want provider %q id evt-explicit", ref, defaultProvider)
	}

	// An explicit id with a flag uses the flag provider.
	ref, err = resolveHold("evt-explicit", "google")
	if err != nil {
		t.Fatalf("resolveHold explicit+flag: %v", err)
	}
	if ref.Provider != "google" {
		t.Errorf("flag provider = %q, want google", ref.Provider)
	}

	// An empty id with no cached hold errors.
	if _, err := resolveHold("", ""); err == nil {
		t.Fatal("resolveHold empty: want error, got nil")
	}

	// An empty id returns the cached hold and its provider.
	want := holdRef{Provider: "graph", ID: "evt-cached"}
	if err := saveState(state{LastHold: want}); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	ref, err = resolveHold("", "")
	if err != nil {
		t.Fatalf("resolveHold cached: %v", err)
	}
	if ref != want {
		t.Errorf("cached ref = %+v, want %+v", ref, want)
	}
}
