package slack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Linker links a Slack user to a calendar provider and mints the credentials a
// command runs with. OAuth providers return a consent URL from AuthURL and finish
// through Exchange; a provider that links another way, such as iCloud by modal,
// returns an empty AuthURL and is completed by the modal submission instead.
type Linker interface {
	// Provider is the calendar provider name this linker handles.
	Provider() string
	// AuthURL returns the OAuth consent URL for the given state and redirect, or an
	// empty string when the provider does not link by web OAuth.
	AuthURL(state, redirectURI string) string
	// Exchange trades an OAuth callback code for the user's stored link.
	Exchange(ctx context.Context, code, redirectURI string) (UserLink, error)
	// RunEnv returns the environment that makes a vamoose subcommand run as the
	// linked user: the provider selection and its credentials.
	RunEnv(ctx context.Context, link UserLink) ([]string, error)
}

// linkState is a pending link that an OAuth callback completes.
type linkState struct {
	// team is the Slack workspace id.
	team string
	// user is the Slack user id.
	user string
	// provider is the calendar provider being linked.
	provider string
	// expiry is when the pending link is no longer valid.
	expiry time.Time
}

// linkStateStore holds short-lived pending links keyed by an opaque state token,
// giving CSRF protection and carrying the workspace, user, and provider through the
// OAuth redirect.
type linkStateStore struct {
	// mu guards states.
	mu sync.Mutex
	// states maps a state token to its pending link.
	states map[string]linkState
	// now supplies the current time, injected for tests.
	now func() time.Time
	// ttl is how long a pending link stays valid.
	ttl time.Duration
}

// newLinkStateStore returns a link-state store using the given clock.
func newLinkStateStore(now func() time.Time) *linkStateStore {
	return &linkStateStore{states: make(map[string]linkState), now: now, ttl: 10 * time.Minute}
}

// issue records a pending link and returns its state token.
func (s *linkStateStore) issue(team, user, provider string) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[tok] = linkState{team: team, user: user, provider: provider, expiry: s.now().Add(s.ttl)}
	return tok
}

// consume validates and removes a state token, returning its pending link.
func (s *linkStateStore) consume(tok string) (linkState, bool) {
	if tok == "" {
		return linkState{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[tok]
	if !ok {
		return linkState{}, false
	}
	delete(s.states, tok)
	if s.now().After(st.expiry) {
		return linkState{}, false
	}
	return st, true
}
