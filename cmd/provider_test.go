package cmd

import (
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TestProviderTokenInjection confirms an injected access token builds a provider
// without the interactive sign-in credentials, and that an unconfigured provider
// still reports its setup error.
func TestProviderTokenInjection(t *testing.T) {
	settings := calendar.Settings{TimeZone: "UTC"}

	// Test 0: An injected Google token builds a provider without a client id.
	t.Run("google injected", func(t *testing.T) {
		t.Setenv("VAMOOSE_GOOGLE_ACCESS_TOKEN", "at-google")
		t.Setenv("VAMOOSE_GOOGLE_CLIENT_ID", "")
		p, err := newGoogleProvider(settings)
		if err != nil || p == nil {
			t.Fatalf("newGoogleProvider = (%v, %v), want provider, nil", p, err)
		}
	})

	// Test 1: Without a token or a client id, Google reports the config error.
	t.Run("google unconfigured", func(t *testing.T) {
		t.Setenv("VAMOOSE_GOOGLE_ACCESS_TOKEN", "")
		t.Setenv("VAMOOSE_GOOGLE_CLIENT_ID", "")
		if _, err := newGoogleProvider(settings); err == nil {
			t.Error("want error when unconfigured, got nil")
		}
	})

	// Test 2: An injected Graph token builds a provider without a client id.
	t.Run("graph injected", func(t *testing.T) {
		t.Setenv("VAMOOSE_GRAPH_ACCESS_TOKEN", "at-graph")
		t.Setenv("VAMOOSE_CLIENT_ID", "")
		p, err := newGraphProvider(settings)
		if err != nil || p == nil {
			t.Fatalf("newGraphProvider = (%v, %v), want provider, nil", p, err)
		}
	})
}
