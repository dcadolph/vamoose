package cmd

import (
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// TestGoogleClientCredsFrom covers credential resolution: the environment override wins
// as an all-or-nothing pair, and the built-in client is the fallback. It mutates the
// package-level built-in vars, so it cannot run in parallel.
func TestGoogleClientCredsFrom(t *testing.T) {
	origID, origSecret := builtinGoogleClientID, builtinGoogleClientSecret
	t.Cleanup(func() { builtinGoogleClientID, builtinGoogleClientSecret = origID, origSecret })

	tests := []struct {
		Name       string
		BuiltinID  string
		BuiltinSec string
		Env        map[string]string
		WantID     string
		WantSecret string
		WantOK     bool
	}{{ // Test 0: No env and no built-in resolves nothing.
		Name: "empty",
	}, { // Test 1: The built-in client is used when the environment is unset.
		Name: "builtin", BuiltinID: "bid", BuiltinSec: "bsec", WantID: "bid", WantSecret: "bsec", WantOK: true,
	}, { // Test 2: A full environment pair overrides the built-in client.
		Name: "env override", BuiltinID: "bid", BuiltinSec: "bsec",
		Env:    map[string]string{"VAMOOSE_GOOGLE_CLIENT_ID": "eid", "VAMOOSE_GOOGLE_CLIENT_SECRET": "esec"},
		WantID: "eid", WantSecret: "esec", WantOK: true,
	}, { // Test 3: A half environment override is not mixed with the built-in secret.
		Name: "env half", BuiltinID: "bid", BuiltinSec: "bsec",
		Env:    map[string]string{"VAMOOSE_GOOGLE_CLIENT_ID": "eid"},
		WantID: "eid", WantSecret: "", WantOK: false,
	}}
	for testNum, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			builtinGoogleClientID, builtinGoogleClientSecret = test.BuiltinID, test.BuiltinSec
			getenv := func(k string) string { return test.Env[k] }
			id, secret, ok := googleClientCredsFrom(getenv)
			if id != test.WantID || secret != test.WantSecret || ok != test.WantOK {
				t.Errorf("test %d: got (%q, %q, %v), want (%q, %q, %v)",
					testNum, id, secret, ok, test.WantID, test.WantSecret, test.WantOK)
			}
		})
	}
}

// TestGoogleProviderBuiltinClient confirms the built-in client builds a provider with no
// environment credentials, the path a fresh `vamoose login` takes. It mutates package
// state, so it cannot run in parallel.
func TestGoogleProviderBuiltinClient(t *testing.T) {
	origID, origSecret := builtinGoogleClientID, builtinGoogleClientSecret
	t.Cleanup(func() { builtinGoogleClientID, builtinGoogleClientSecret = origID, origSecret })
	builtinGoogleClientID, builtinGoogleClientSecret = "bid", "bsec"
	t.Setenv("VAMOOSE_GOOGLE_ACCESS_TOKEN", "")
	t.Setenv("VAMOOSE_GOOGLE_CLIENT_ID", "")
	t.Setenv("VAMOOSE_GOOGLE_CLIENT_SECRET", "")

	p, err := newGoogleProvider(calendar.Settings{TimeZone: "UTC"})
	if err != nil || p == nil {
		t.Fatalf("newGoogleProvider with built-in client = (%v, %v), want provider, nil", p, err)
	}
}
