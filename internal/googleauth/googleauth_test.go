package googleauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/auth"
)

// memStore is an in-memory TokenStore for tests.
type memStore struct {
	// tok is the stored token.
	tok auth.Token
}

func (m *memStore) Load() (auth.Token, error) { return m.tok, nil }
func (m *memStore) Save(t auth.Token) error   { m.tok = t; return nil }

// tokenHandler returns a handler that asserts the grant and replies with a token.
func tokenHandler(t *testing.T, wantGrant string, refresh string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.PostForm.Get("grant_type"); got != wantGrant {
			t.Errorf("grant_type = %q, want %q", got, wantGrant)
		}
		w.Header().Set("Content-Type", "application/json")
		body := `{"access_token":"server-token","expires_in":3600,"scope":"calendar"`
		if refresh != "" {
			body += `,"refresh_token":"` + refresh + `"`
		}
		body += `}`
		_, _ = w.Write([]byte(body))
	}
}

// TestAuthenticatorLoopback drives the full loopback flow with a fake browser
// that simulates Google's redirect back to the loopback listener.
func TestAuthenticatorLoopback(t *testing.T) {
	t.Parallel()
	var gotCode, gotVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotCode = r.PostForm.Get("code")
		gotVerifier = r.PostForm.Get("code_verifier")
		if r.PostForm.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q, want authorization_code", r.PostForm.Get("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"server-token","refresh_token":"r1","expires_in":3600}`))
	}))
	defer srv.Close()

	browser := func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		redirect := q.Get("redirect_uri") + "?code=fake-code&state=" + q.Get("state")
		go func() {
			resp, gerr := http.Get(redirect) //nolint:noctx // Test-only redirect simulation.
			if gerr == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}

	store := &memStore{}
	a := NewAuthenticator("cid", "secret", store,
		WithTokenURL(srv.URL),
		WithAuthURL("http://auth.example/authorize"),
		WithBrowser(browser),
	)
	tok, err := a.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "server-token" {
		t.Errorf("access token = %q, want server-token", tok.AccessToken)
	}
	if gotCode != "fake-code" {
		t.Errorf("exchanged code = %q, want fake-code", gotCode)
	}
	if gotVerifier == "" {
		t.Error("code_verifier was not sent")
	}
	if store.tok.AccessToken != "server-token" {
		t.Errorf("stored token = %q, want server-token", store.tok.AccessToken)
	}
}

// TestAuthenticatorRefresh confirms a valid refresh token skips the browser and
// carries forward when Google omits it in the reply.
func TestAuthenticatorRefresh(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(tokenHandler(t, "refresh_token", ""))
	defer srv.Close()

	browserCalled := false
	store := &memStore{tok: auth.Token{RefreshToken: "r1", Expiry: time.Now().Add(-time.Hour)}}
	a := NewAuthenticator("cid", "secret", store,
		WithTokenURL(srv.URL),
		WithBrowser(func(string) error { browserCalled = true; return nil }),
	)
	tok, err := a.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if browserCalled {
		t.Error("browser opened despite a valid refresh token")
	}
	if tok.AccessToken != "server-token" {
		t.Errorf("access token = %q, want server-token", tok.AccessToken)
	}
	if tok.RefreshToken != "r1" {
		t.Errorf("refresh token = %q, want carried-forward r1", tok.RefreshToken)
	}
}

func TestAuthCodeURL(t *testing.T) {
	t.Parallel()
	a := NewAuthenticator("cid", "secret", &memStore{})
	raw := a.authCodeURL("http://127.0.0.1:9999", "chal", "st8")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	checks := map[string]string{
		"client_id":             "cid",
		"redirect_uri":          "http://127.0.0.1:9999",
		"response_type":         "code",
		"code_challenge":        "chal",
		"code_challenge_method": "S256",
		"state":                 "st8",
		"access_type":           "offline",
	}
	for key, want := range checks {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
