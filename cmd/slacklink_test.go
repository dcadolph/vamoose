package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/dcadolph/vamoose/internal/auth"
	"github.com/dcadolph/vamoose/internal/googleauth"
)

// TestGoogleLinker confirms the linker exchanges a code for a refresh token and
// refreshes it into the environment that runs a command as that Google user.
func TestGoogleLinker(t *testing.T) {
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

	l := &googleLinker{auth: googleauth.NewAuthenticator("id", "secret", noopTokenStore{}, googleauth.WithTokenURL(srv.URL))}

	if l.Provider() != providerGoogle {
		t.Errorf("Provider = %q, want %q", l.Provider(), providerGoogle)
	}

	// Exchange yields a link holding the refresh token.
	link, err := l.Exchange(context.Background(), "code", "https://pub/cb")
	if err != nil || link.Provider != providerGoogle || link.RefreshToken != "r1" {
		t.Fatalf("Exchange = %+v, %v; want google/r1", link, err)
	}

	// RunEnv refreshes to an access token and returns the injection environment.
	env, err := l.RunEnv(context.Background(), link)
	if err != nil {
		t.Fatalf("RunEnv: %v", err)
	}
	if !slices.Contains(env, "VAMOOSE_PROVIDER=google") || !slices.Contains(env, "VAMOOSE_GOOGLE_ACCESS_TOKEN=a2") {
		t.Errorf("RunEnv env = %v, want provider google + access a2", env)
	}
}

// TestGraphLinker confirms the Graph linker exchanges a code and refreshes it into
// the environment that runs a command as that Microsoft 365 user.
func TestGraphLinker(t *testing.T) {
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

	l := &graphLinker{auth: auth.NewAuthenticator("t", "cid", noopTokenStore{}, auth.WithClientSecret("sec"), auth.WithBaseURL(srv.URL))}

	if l.Provider() != defaultProvider {
		t.Errorf("Provider = %q, want %q", l.Provider(), defaultProvider)
	}
	link, err := l.Exchange(context.Background(), "code", "https://pub/cb")
	if err != nil || link.RefreshToken != "r1" {
		t.Fatalf("Exchange = %+v, %v; want refresh r1", link, err)
	}
	env, err := l.RunEnv(context.Background(), link)
	if err != nil {
		t.Fatalf("RunEnv: %v", err)
	}
	if !slices.Contains(env, "VAMOOSE_PROVIDER=graph") || !slices.Contains(env, "VAMOOSE_GRAPH_ACCESS_TOKEN=a2") {
		t.Errorf("RunEnv env = %v, want provider graph + access a2", env)
	}
}
