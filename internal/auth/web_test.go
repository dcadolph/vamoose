package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stubStore is a TokenStore that persists nothing, for the web-flow tests.
type stubStore struct{}

func (stubStore) Load() (Token, error) { return Token{}, nil }
func (stubStore) Save(Token) error     { return nil }

// TestWebAuthCodeURL confirms the web consent URL targets the authorize endpoint
// with the code flow and state.
func TestWebAuthCodeURL(t *testing.T) {
	t.Parallel()
	a := NewAuthenticator("mytenant", "cid", stubStore{}, WithClientSecret("sec"))
	u, err := url.Parse(a.WebAuthCodeURL("https://pub/cb", "st8"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(u.Path, "/mytenant/oauth2/v2.0/authorize") {
		t.Errorf("path = %q, want the authorize endpoint", u.Path)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"client_id": "cid", "response_type": "code", "redirect_uri": "https://pub/cb", "state": "st8",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
}

// TestExchangeAndRefresh confirms the web code exchange and refresh return tokens,
// carrying the refresh token forward when omitted.
func TestExchangeAndRefresh(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			_, _ = w.Write([]byte(`{"access_token":"a1","refresh_token":"r1","expires_in":3600}`))
		case "refresh_token":
			_, _ = w.Write([]byte(`{"access_token":"a2","expires_in":3600}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	a := NewAuthenticator("t", "cid", stubStore{}, WithClientSecret("sec"), WithBaseURL(srv.URL))

	tok, err := a.ExchangeCode(context.Background(), "code", "https://pub/cb")
	if err != nil || tok.AccessToken != "a1" || tok.RefreshToken != "r1" {
		t.Fatalf("ExchangeCode = %+v, %v; want a1/r1", tok, err)
	}
	rt, err := a.Refresh(context.Background(), "r1")
	if err != nil || rt.AccessToken != "a2" || rt.RefreshToken != "r1" {
		t.Errorf("Refresh = %+v, %v; want a2 with carried r1", rt, err)
	}
}
