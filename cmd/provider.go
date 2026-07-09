package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/dcadolph/vamoose/internal/auth"
	"github.com/dcadolph/vamoose/internal/caldav"
	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/eventkit"
	"github.com/dcadolph/vamoose/internal/google"
	"github.com/dcadolph/vamoose/internal/googleauth"
	"github.com/dcadolph/vamoose/internal/graph"
)

const (
	// defaultProvider is the calendar provider used when none is selected.
	defaultProvider = "graph"
	// envProvider is the environment variable that selects the calendar provider.
	envProvider = "VAMOOSE_PROVIDER"
	// providerGoogle is the Google Calendar provider name.
	providerGoogle = "google"
	// providerICloud is the Apple iCloud CalDAV provider name.
	providerICloud = "icloud"
)

// newRegistry returns a provider registry with the built-in providers registered.
func newRegistry() *calendar.Registry {
	r := calendar.NewRegistry()
	r.Register(defaultProvider, newGraphProvider)
	r.Register(providerGoogle, newGoogleProvider)
	r.Register(providerICloud, newICloudProvider)
	return r
}

// newProvider builds the named calendar provider with the given time zone.
func newProvider(name, tz string) (calendar.Provider, error) {
	return newRegistry().Build(name, calendar.Settings{TimeZone: tz})
}

// resolveProvider returns the provider name from the flag, the environment, or
// the built-in default, in that order.
func resolveProvider(flagProvider string) string {
	if flagProvider != "" {
		return flagProvider
	}
	if env := os.Getenv(envProvider); env != "" {
		return env
	}
	return defaultProvider
}

// newGraphProvider builds a Microsoft Graph provider from environment settings.
func newGraphProvider(s calendar.Settings) (calendar.Provider, error) {
	clientID := os.Getenv("VAMOOSE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("VAMOOSE_CLIENT_ID not set: register an Entra app and export its client id")
	}
	tenant := os.Getenv("VAMOOSE_TENANT")
	if tenant == "" {
		tenant = "organizations"
	}
	store, err := auth.NewStore(defaultProvider)
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
	return graph.NewProvider(graph.TokenSource(source), graph.WithTimeZone(s.TimeZone)), nil
}

// newGoogleProvider builds a Google Calendar provider from environment settings.
func newGoogleProvider(s calendar.Settings) (calendar.Provider, error) {
	clientID := os.Getenv("VAMOOSE_GOOGLE_CLIENT_ID")
	if clientID == "" {
		return nil, fmt.Errorf("VAMOOSE_GOOGLE_CLIENT_ID not set: create an OAuth desktop client and export its id")
	}
	clientSecret := os.Getenv("VAMOOSE_GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		return nil, fmt.Errorf("VAMOOSE_GOOGLE_CLIENT_SECRET not set: export the OAuth desktop client secret")
	}
	store, err := auth.NewStore(providerGoogle)
	if err != nil {
		return nil, fmt.Errorf("token store: %w", err)
	}
	authr := googleauth.NewAuthenticator(clientID, clientSecret, store, googleauth.WithPrompt(os.Stderr))
	source := func(ctx context.Context) (string, error) {
		tok, terr := authr.Token(ctx)
		if terr != nil {
			return "", terr
		}
		return tok.AccessToken, nil
	}
	return google.NewProvider(google.TokenSource(source), google.WithTimeZone(s.TimeZone)), nil
}

// newICloudProvider builds an Apple iCloud CalDAV provider from environment settings.
func newICloudProvider(s calendar.Settings) (calendar.Provider, error) {
	user := os.Getenv("VAMOOSE_ICLOUD_USERNAME")
	if user == "" {
		return nil, fmt.Errorf("VAMOOSE_ICLOUD_USERNAME not set: your Apple ID email")
	}
	pass := os.Getenv("VAMOOSE_ICLOUD_APP_PASSWORD")
	if pass == "" {
		return nil, fmt.Errorf("VAMOOSE_ICLOUD_APP_PASSWORD not set: create an app-specific password at appleid.apple.com")
	}
	opts := []caldav.Option{caldav.WithTimeZone(s.TimeZone)}
	if name := os.Getenv("VAMOOSE_ICLOUD_CALENDAR"); name != "" {
		opts = append(opts, caldav.WithCalendarName(name))
	}
	// On macOS, recover attendee accept/decline from the local Calendar.app, which
	// iCloud does not report over CalDAV.
	if runtime.GOOS == "darwin" {
		opts = append(opts, caldav.WithStatus(eventkit.Status))
	}
	return caldav.NewProvider("https://caldav.icloud.com", user, pass, opts...)
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
