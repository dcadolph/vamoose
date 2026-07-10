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
	// providerCalDAV is the generic CalDAV provider name for any standard host.
	providerCalDAV = "caldav"
)

// newRegistry returns a provider registry with the built-in providers registered.
func newRegistry() *calendar.Registry {
	r := calendar.NewRegistry()
	r.Register(defaultProvider, newGraphProvider)
	r.Register(providerGoogle, newGoogleProvider)
	r.Register(providerICloud, newICloudProvider)
	r.Register(providerCalDAV, newCalDAVProvider)
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
// When VAMOOSE_GRAPH_ACCESS_TOKEN is set, it uses that token directly, which lets
// a caller such as the Slack server run a command as a specific linked user without
// the interactive sign-in flow.
func newGraphProvider(s calendar.Settings) (calendar.Provider, error) {
	if tok := os.Getenv("VAMOOSE_GRAPH_ACCESS_TOKEN"); tok != "" {
		source := func(context.Context) (string, error) { return tok, nil }
		return graph.NewProvider(graph.TokenSource(source), graph.WithTimeZone(s.TimeZone)), nil
	}
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
// When VAMOOSE_GOOGLE_ACCESS_TOKEN is set, it uses that token directly, which lets
// a caller such as the Slack server run a command as a specific linked user without
// the interactive sign-in flow.
func newGoogleProvider(s calendar.Settings) (calendar.Provider, error) {
	if tok := os.Getenv("VAMOOSE_GOOGLE_ACCESS_TOKEN"); tok != "" {
		source := func(context.Context) (string, error) { return tok, nil }
		return google.NewProvider(google.TokenSource(source), google.WithTimeZone(s.TimeZone)), nil
	}
	clientID, clientSecret, ok := googleClientCreds()
	if !ok {
		return nil, fmt.Errorf("no Google client configured: run 'vamoose login', or set VAMOOSE_GOOGLE_CLIENT_ID and VAMOOSE_GOOGLE_CLIENT_SECRET")
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

// newCalDAVProvider builds a generic CalDAV provider for any standard host, such as
// Fastmail or Nextcloud, from environment settings. Unlike iCloud, a standard host
// reports attendee accept and decline over CalDAV, so approval detection and the
// daemon work without a local helper.
func newCalDAVProvider(s calendar.Settings) (calendar.Provider, error) {
	endpoint := os.Getenv("VAMOOSE_CALDAV_URL")
	if endpoint == "" {
		return nil, fmt.Errorf("VAMOOSE_CALDAV_URL not set: your CalDAV server URL, such as https://caldav.fastmail.com")
	}
	user := os.Getenv("VAMOOSE_CALDAV_USERNAME")
	if user == "" {
		return nil, fmt.Errorf("VAMOOSE_CALDAV_USERNAME not set: your CalDAV account username")
	}
	pass := os.Getenv("VAMOOSE_CALDAV_PASSWORD")
	if pass == "" {
		return nil, fmt.Errorf("VAMOOSE_CALDAV_PASSWORD not set: your CalDAV account password or app-specific password")
	}
	opts := []caldav.Option{caldav.WithTimeZone(s.TimeZone)}
	if name := os.Getenv("VAMOOSE_CALDAV_CALENDAR"); name != "" {
		opts = append(opts, caldav.WithCalendarName(name))
	}
	return caldav.NewProvider(endpoint, user, pass, opts...)
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
