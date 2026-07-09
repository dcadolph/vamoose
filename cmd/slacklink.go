package cmd

import (
	"context"
	"errors"

	"github.com/dcadolph/vamoose/internal/auth"
	"github.com/dcadolph/vamoose/internal/googleauth"
	"github.com/dcadolph/vamoose/internal/slack"
)

// noopTokenStore satisfies auth.TokenStore for OAuth flows that persist their own
// refresh token rather than using the process token store. The Slack per-user web
// flow stores each user's refresh token in the link store, so the authenticator's
// own store is never used.
type noopTokenStore struct{}

// Load returns an empty token.
func (noopTokenStore) Load() (auth.Token, error) { return auth.Token{}, nil }

// Save discards the token.
func (noopTokenStore) Save(auth.Token) error { return nil }

// googleLinker links a Slack user's Google Calendar through a server-side web OAuth
// flow and mints a fresh access token for each command that user runs.
type googleLinker struct {
	// auth drives the Google web OAuth flow and token refresh.
	auth *googleauth.Authenticator
}

// newGoogleLinker builds a Google linker from OAuth web-client credentials.
func newGoogleLinker(clientID, clientSecret string) *googleLinker {
	return &googleLinker{auth: googleauth.NewAuthenticator(clientID, clientSecret, noopTokenStore{})}
}

// Provider returns the calendar provider name.
func (l *googleLinker) Provider() string { return providerGoogle }

// AuthURL returns the Google consent URL for the web flow.
func (l *googleLinker) AuthURL(state, redirectURI string) string {
	return l.auth.WebAuthCodeURL(redirectURI, state)
}

// Exchange trades the callback code for a link holding the user's refresh token.
func (l *googleLinker) Exchange(ctx context.Context, code, redirectURI string) (slack.UserLink, error) {
	tok, err := l.auth.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		return slack.UserLink{}, err
	}
	return slack.UserLink{Provider: providerGoogle, RefreshToken: tok.RefreshToken}, nil
}

// RunEnv refreshes the user's access token and returns the environment that runs a
// vamoose subcommand as that Google user.
func (l *googleLinker) RunEnv(ctx context.Context, link slack.UserLink) ([]string, error) {
	tok, err := l.auth.Refresh(ctx, link.RefreshToken)
	if err != nil {
		return nil, err
	}
	return []string{
		"VAMOOSE_PROVIDER=" + providerGoogle,
		"VAMOOSE_GOOGLE_ACCESS_TOKEN=" + tok.AccessToken,
	}, nil
}

// graphLinker links a Slack user's Microsoft 365 calendar through a server-side web
// OAuth flow and mints a fresh access token for each command that user runs.
type graphLinker struct {
	// auth drives the Graph web OAuth flow and token refresh.
	auth *auth.Authenticator
}

// newGraphLinker builds a Graph linker from a confidential web-client registration.
func newGraphLinker(tenant, clientID, clientSecret string) *graphLinker {
	return &graphLinker{auth: auth.NewAuthenticator(tenant, clientID, noopTokenStore{}, auth.WithClientSecret(clientSecret))}
}

// Provider returns the calendar provider name.
func (l *graphLinker) Provider() string { return defaultProvider }

// AuthURL returns the Microsoft consent URL for the web flow.
func (l *graphLinker) AuthURL(state, redirectURI string) string {
	return l.auth.WebAuthCodeURL(redirectURI, state)
}

// Exchange trades the callback code for a link holding the user's refresh token.
func (l *graphLinker) Exchange(ctx context.Context, code, redirectURI string) (slack.UserLink, error) {
	tok, err := l.auth.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		return slack.UserLink{}, err
	}
	return slack.UserLink{Provider: defaultProvider, RefreshToken: tok.RefreshToken}, nil
}

// RunEnv refreshes the user's access token and returns the environment that runs a
// vamoose subcommand as that Microsoft 365 user.
func (l *graphLinker) RunEnv(ctx context.Context, link slack.UserLink) ([]string, error) {
	tok, err := l.auth.Refresh(ctx, link.RefreshToken)
	if err != nil {
		return nil, err
	}
	return []string{
		"VAMOOSE_PROVIDER=" + defaultProvider,
		"VAMOOSE_GRAPH_ACCESS_TOKEN=" + tok.AccessToken,
	}, nil
}

// icloudLinker links a Slack user's iCloud calendar. iCloud has no OAuth, so the
// user submits an Apple ID and app-specific password through a Slack modal; those
// credentials are stored and injected per command.
type icloudLinker struct{}

// Provider returns the calendar provider name.
func (icloudLinker) Provider() string { return providerICloud }

// AuthURL returns an empty string, signaling that iCloud links by modal, not OAuth.
func (icloudLinker) AuthURL(string, string) string { return "" }

// Exchange is unused for iCloud; the modal submission builds the link directly.
func (icloudLinker) Exchange(context.Context, string, string) (slack.UserLink, error) {
	return slack.UserLink{}, errors.New("icloud links via a modal, not oauth")
}

// RunEnv returns the environment that runs a vamoose subcommand as the linked
// iCloud user.
func (icloudLinker) RunEnv(_ context.Context, link slack.UserLink) ([]string, error) {
	if link.ICloudUser == "" || link.ICloudAppPassword == "" {
		return nil, errors.New("icloud link is missing credentials")
	}
	return []string{
		"VAMOOSE_PROVIDER=" + providerICloud,
		"VAMOOSE_ICLOUD_USERNAME=" + link.ICloudUser,
		"VAMOOSE_ICLOUD_APP_PASSWORD=" + link.ICloudAppPassword,
	}, nil
}
