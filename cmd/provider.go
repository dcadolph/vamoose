package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dcadolph/vamoose/internal/auth"
	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/graph"
)

// newProvider builds a Graph-backed calendar.Provider from environment settings.
func newProvider(tz string) (calendar.Provider, error) {
	clientID := os.Getenv("VAMOOSE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("VAMOOSE_CLIENT_ID not set: register an Entra app and export its client id")
	}
	tenant := os.Getenv("VAMOOSE_TENANT")
	if tenant == "" {
		tenant = "organizations"
	}
	store, err := auth.NewFileStore()
	if err != nil {
		return nil, fmt.Errorf("token store: %w", err)
	}
	authr := auth.NewAuthenticator(tenant, clientID, store, auth.WithPrompt(os.Stderr))
	source := func(ctx context.Context) (string, error) {
		tok, terr := authr.Token(ctx)
		if terr != nil {
			return "", terr
		}
		return tok.AccessToken, nil
	}
	return graph.NewProvider(graph.TokenSource(source), graph.WithTimeZone(tz)), nil
}

// resolveTimeZone returns the configured IANA zone, defaulting to UTC.
func resolveTimeZone(flagTZ string) string {
	if flagTZ != "" {
		return flagTZ
	}
	if env := os.Getenv("VAMOOSE_TIMEZONE"); env != "" {
		return env
	}
	return "UTC"
}
