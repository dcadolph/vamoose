package cmd

import (
	"context"

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
