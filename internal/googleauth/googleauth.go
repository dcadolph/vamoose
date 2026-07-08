// Package googleauth obtains and caches Google OAuth tokens using the loopback
// redirect flow with PKCE, the recommended grant for installed desktop apps.
// It reuses the provider-neutral token types from the auth package.
package googleauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dcadolph/vamoose/internal/auth"
)

const (
	// authEndpoint is Google's OAuth 2.0 authorization endpoint.
	authEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	// tokenEndpoint is Google's OAuth 2.0 token endpoint.
	tokenEndpoint = "https://oauth2.googleapis.com/token"
)

// DefaultScopes are the delegated scopes vamoose needs: read/write calendar
// events, and read access to resolve the primary calendar's owner.
var DefaultScopes = []string{
	"https://www.googleapis.com/auth/calendar.events",
	"https://www.googleapis.com/auth/calendar.readonly",
}

// Authenticator obtains and refreshes Google OAuth tokens via the loopback flow.
type Authenticator struct {
	// clientID is the OAuth desktop client id.
	clientID string
	// clientSecret is the OAuth desktop client secret.
	clientSecret string
	// scopes are the delegated permission scopes to request.
	scopes []string
	// store persists tokens between runs.
	store auth.TokenStore
	// client issues HTTP requests to the token endpoint.
	client *http.Client
	// prompt receives the human-readable authorization URL.
	prompt io.Writer
	// openBrowser launches the user's browser at the authorization URL.
	openBrowser func(string) error
	// authURL is the authorization endpoint, overridable for testing.
	authURL string
	// tokenURL is the token endpoint, overridable for testing.
	tokenURL string
}

// Option configures an Authenticator.
type Option func(*Authenticator)

// WithHTTPClient sets the HTTP client used for token requests.
func WithHTTPClient(c *http.Client) Option { return func(a *Authenticator) { a.client = c } }

// WithPrompt sets the writer that receives the authorization URL.
func WithPrompt(w io.Writer) Option { return func(a *Authenticator) { a.prompt = w } }

// WithScopes overrides the default delegated scopes.
func WithScopes(scopes ...string) Option { return func(a *Authenticator) { a.scopes = scopes } }

// WithBrowser sets the function that opens the authorization URL.
func WithBrowser(open func(string) error) Option {
	return func(a *Authenticator) { a.openBrowser = open }
}

// WithAuthURL overrides the authorization endpoint, mainly for testing.
func WithAuthURL(u string) Option { return func(a *Authenticator) { a.authURL = u } }

// WithTokenURL overrides the token endpoint, mainly for testing.
func WithTokenURL(u string) Option { return func(a *Authenticator) { a.tokenURL = u } }

// NewAuthenticator returns an Authenticator for the given desktop client.
// It panics if clientID, clientSecret, or store is missing, signaling developer error.
func NewAuthenticator(clientID, clientSecret string, store auth.TokenStore, opts ...Option) *Authenticator {
	if clientID == "" || clientSecret == "" {
		panic("googleauth.NewAuthenticator: clientID and clientSecret required")
	}
	if store == nil {
		panic("googleauth.NewAuthenticator: store required")
	}
	a := &Authenticator{
		clientID:     clientID,
		clientSecret: clientSecret,
		scopes:       DefaultScopes,
		store:        store,
		client:       &http.Client{Timeout: 30 * time.Second},
		prompt:       io.Discard,
		openBrowser:  openBrowser,
		authURL:      authEndpoint,
		tokenURL:     tokenEndpoint,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Token returns a valid access token, refreshing or running the loopback flow
// as needed, and persists any newly obtained token.
func (a *Authenticator) Token(ctx context.Context) (auth.Token, error) {
	tok, err := a.store.Load()
	if err != nil {
		return auth.Token{}, fmt.Errorf("load token: %w", err)
	}
	if tok.Valid() {
		return tok, nil
	}
	if tok.RefreshToken != "" {
		if refreshed, rerr := a.refresh(ctx, tok.RefreshToken); rerr == nil {
			if serr := a.store.Save(refreshed); serr != nil {
				return auth.Token{}, fmt.Errorf("save token: %w", serr)
			}
			return refreshed, nil
		}
		// Fall through to the interactive flow when refresh fails.
	}
	fresh, err := a.loopback(ctx)
	if err != nil {
		return auth.Token{}, err
	}
	if err := a.store.Save(fresh); err != nil {
		return auth.Token{}, fmt.Errorf("save token: %w", err)
	}
	return fresh, nil
}

// loopback runs the interactive loopback flow: it opens the browser to the
// consent screen and exchanges the returned code for tokens.
func (a *Authenticator) loopback(ctx context.Context) (auth.Token, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return auth.Token{}, fmt.Errorf("start loopback listener: %w", err)
	}
	defer func() { _ = ln.Close() }()
	redirectURI := "http://" + ln.Addr().String()

	verifier, challenge, err := pkce()
	if err != nil {
		return auth.Token{}, err
	}
	state, err := randomState()
	if err != nil {
		return auth.Token{}, err
	}

	authURL := a.authCodeURL(redirectURI, challenge, state)
	fmt.Fprintf(a.prompt, "Open this URL to authorize vamoose:\n%s\n", authURL)
	if a.openBrowser != nil {
		_ = a.openBrowser(authURL)
	}

	code, err := a.waitForCode(ctx, ln, state)
	if err != nil {
		return auth.Token{}, err
	}
	return a.exchange(ctx, code, verifier, redirectURI)
}

// waitForCode serves the loopback listener until the consent redirect arrives,
// returning the authorization code or an error.
func (a *Authenticator) waitForCode(ctx context.Context, ln net.Listener, wantState string) (string, error) {
	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)
	var once sync.Once
	send := func(r result) { once.Do(func() { ch <- r }) }

	srv := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			switch {
			case q.Get("error") != "":
				writeFailure(w)
				send(result{err: fmt.Errorf("%w: %s", ErrAuthorization, q.Get("error"))})
			case q.Get("state") != wantState:
				writeFailure(w)
				send(result{err: ErrState})
			case q.Get("code") == "":
				writeFailure(w)
				send(result{err: ErrNoCode})
			default:
				writeSuccess(w)
				send(result{code: q.Get("code")})
			}
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.code, res.err
	}
}

// exchange trades an authorization code and PKCE verifier for tokens.
func (a *Authenticator) exchange(ctx context.Context, code, verifier, redirectURI string) (auth.Token, error) {
	form := url.Values{
		"client_id":     {a.clientID},
		"client_secret": {a.clientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}
	return a.postToken(ctx, form)
}

// refresh exchanges a refresh token for a new access token. Google omits the
// refresh token on refresh, so the caller's token is carried forward.
func (a *Authenticator) refresh(ctx context.Context, refreshToken string) (auth.Token, error) {
	form := url.Values{
		"client_id":     {a.clientID},
		"client_secret": {a.clientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}
	t, err := a.postToken(ctx, form)
	if err != nil {
		return auth.Token{}, err
	}
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

// tokenResponse is Google's token endpoint reply.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// postToken posts a URL-encoded form to the token endpoint and builds a Token.
func (a *Authenticator) postToken(ctx context.Context, form url.Values) (auth.Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return auth.Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return auth.Token{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return auth.Token{}, err
	}
	var tr tokenResponse
	if err := json.Unmarshal(b, &tr); err != nil {
		return auth.Token{}, err
	}
	if tr.Error != "" {
		return auth.Token{}, fmt.Errorf("%w: %s: %s", ErrToken, tr.Error, tr.ErrorDescription)
	}
	if tr.AccessToken == "" {
		return auth.Token{}, fmt.Errorf("%w: empty access token", ErrToken)
	}
	return auth.Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Scope:        tr.Scope,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

// authCodeURL builds the consent screen URL with PKCE and offline access.
func (a *Authenticator) authCodeURL(redirectURI, challenge, state string) string {
	q := url.Values{
		"client_id":             {a.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(a.scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}
	return a.authURL + "?" + q.Encode()
}

// pkce returns a PKCE code verifier and its S256 challenge.
func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// randomState returns a random anti-forgery state value.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// openBrowser launches the platform browser at target on a best-effort basis.
func openBrowser(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

// successPage is shown in the browser after a successful authorization.
const successPage = `<!doctype html>
<title>vamoose</title>
<body style="font-family:sans-serif;text-align:center;margin-top:4rem">
<h2>vamoose is authorized</h2>
<p>You can close this window.</p>
</body>`

// failurePage is shown in the browser after a failed authorization.
const failurePage = `<!doctype html>
<title>vamoose</title>
<body style="font-family:sans-serif;text-align:center;margin-top:4rem">
<h2>Authorization failed</h2>
<p>Return to vamoose and try again.</p>
</body>`

// writeSuccess writes the success page.
func writeSuccess(w http.ResponseWriter) { writeHTML(w, successPage) }

// writeFailure writes the failure page.
func writeFailure(w http.ResponseWriter) { writeHTML(w, failurePage) }

// writeHTML writes a static HTML page.
func writeHTML(w http.ResponseWriter, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, page)
}
