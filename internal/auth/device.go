package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// authHost is the Microsoft identity platform host.
const authHost = "https://login.microsoftonline.com"

// DefaultScopes are the delegated scopes vamoose needs for the full flow:
// reading the directory for manager and team, read/write calendar, and
// read/write mailbox settings for the optional out-of-office reply.
var DefaultScopes = []string{
	"offline_access",
	"User.Read",
	"User.Read.All",
	"Calendars.ReadWrite",
	"MailboxSettings.ReadWrite",
}

// Authenticator obtains and refreshes Microsoft Graph tokens via the OAuth 2.0
// device authorization grant.
type Authenticator struct {
	// tenant is the Entra tenant id, "common", or "organizations".
	tenant string
	// clientID is the registered Entra application (client) id.
	clientID string
	// scopes are the delegated permission scopes to request.
	scopes []string
	// store persists tokens between runs.
	store TokenStore
	// client issues HTTP requests to the identity platform.
	client *http.Client
	// prompt receives human-readable device-code instructions.
	prompt io.Writer
	// clientSecret is the confidential-client secret, set to enable the server-side
	// web authorization-code flow.
	clientSecret string
	// host is the identity platform base URL, overridable for tests.
	host string
}

// Option configures an Authenticator.
type Option func(*Authenticator)

// WithHTTPClient sets the HTTP client used for token requests.
func WithHTTPClient(c *http.Client) Option { return func(a *Authenticator) { a.client = c } }

// WithPrompt sets the writer that receives device-code instructions.
func WithPrompt(w io.Writer) Option { return func(a *Authenticator) { a.prompt = w } }

// WithScopes overrides the default delegated scopes.
func WithScopes(scopes ...string) Option { return func(a *Authenticator) { a.scopes = scopes } }

// WithClientSecret sets the confidential-client secret, enabling the server-side
// web authorization-code flow used by WebAuthCodeURL and ExchangeCode.
func WithClientSecret(secret string) Option {
	return func(a *Authenticator) { a.clientSecret = secret }
}

// WithBaseURL overrides the identity platform base URL, for tests.
func WithBaseURL(u string) Option { return func(a *Authenticator) { a.host = u } }

// NewAuthenticator returns an Authenticator for the given tenant and client id.
// It panics if tenant, clientID, or store is missing, signaling developer error.
func NewAuthenticator(tenant, clientID string, store TokenStore, opts ...Option) *Authenticator {
	if tenant == "" || clientID == "" {
		panic("auth.NewAuthenticator: tenant and clientID required")
	}
	if store == nil {
		panic("auth.NewAuthenticator: store required")
	}
	a := &Authenticator{
		tenant:   tenant,
		clientID: clientID,
		scopes:   DefaultScopes,
		store:    store,
		client:   &http.Client{Timeout: 30 * time.Second},
		prompt:   io.Discard,
		host:     authHost,
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Token returns a valid access token, refreshing or running the device-code
// flow as needed, and persists any newly obtained token.
func (a *Authenticator) Token(ctx context.Context) (Token, error) {
	tok, err := a.store.Load()
	if err != nil {
		return Token{}, fmt.Errorf("load token: %w", err)
	}
	if tok.Valid() {
		return tok, nil
	}
	if tok.RefreshToken != "" {
		if refreshed, rerr := a.refresh(ctx, tok.RefreshToken); rerr == nil {
			if serr := a.store.Save(refreshed); serr != nil {
				return Token{}, fmt.Errorf("save token: %w", serr)
			}
			return refreshed, nil
		}
		// Fall through to the interactive flow when refresh fails.
	}
	fresh, err := a.device(ctx)
	if err != nil {
		return Token{}, err
	}
	if err := a.store.Save(fresh); err != nil {
		return Token{}, fmt.Errorf("save token: %w", err)
	}
	return fresh, nil
}

// deviceCodeResponse is the identity platform's device authorization reply.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// tokenResponse is the identity platform's token reply.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// device runs the interactive device-code flow, prompting the user to sign in.
func (a *Authenticator) device(ctx context.Context) (Token, error) {
	dc, err := a.requestDeviceCode(ctx)
	if err != nil {
		return Token{}, err
	}
	if dc.Message != "" {
		fmt.Fprintln(a.prompt, dc.Message)
	} else {
		fmt.Fprintf(a.prompt, "Open %s and enter code %s\n", dc.VerificationURI, dc.UserCode)
	}
	return a.pollToken(ctx, dc)
}

// requestDeviceCode asks the identity platform for a device and user code.
func (a *Authenticator) requestDeviceCode(ctx context.Context) (deviceCodeResponse, error) {
	form := url.Values{
		"client_id": {a.clientID},
		"scope":     {strings.Join(a.scopes, " ")},
	}
	var dc deviceCodeResponse
	if err := a.postForm(ctx, a.deviceEndpoint(), form, &dc); err != nil {
		return deviceCodeResponse{}, fmt.Errorf("device code request: %w", err)
	}
	if dc.DeviceCode == "" {
		return deviceCodeResponse{}, ErrDeviceCode
	}
	return dc, nil
}

// pollToken polls the token endpoint until the user authorizes or time runs out.
func (a *Authenticator) pollToken(ctx context.Context, dc deviceCodeResponse) (Token, error) {
	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	form := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {a.clientID},
		"device_code": {dc.DeviceCode},
	}
	for {
		if time.Now().After(deadline) {
			return Token{}, ErrDeviceExpired
		}
		select {
		case <-ctx.Done():
			return Token{}, ctx.Err()
		case <-time.After(interval):
		}
		var tr tokenResponse
		if err := a.postForm(ctx, a.tokenEndpoint(), form, &tr); err != nil {
			return Token{}, err
		}
		switch tr.Error {
		case "":
			return tokenFrom(tr), nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
		case "authorization_declined":
			return Token{}, ErrDeviceDeclined
		case "expired_token":
			return Token{}, ErrDeviceExpired
		default:
			return Token{}, fmt.Errorf("%w: %s: %s", ErrToken, tr.Error, tr.ErrorDescription)
		}
	}
}

// refresh exchanges a refresh token for a new access token.
func (a *Authenticator) refresh(ctx context.Context, refreshToken string) (Token, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {a.clientID},
		"refresh_token": {refreshToken},
		"scope":         {strings.Join(a.scopes, " ")},
	}
	if a.clientSecret != "" {
		form.Set("client_secret", a.clientSecret)
	}
	var tr tokenResponse
	if err := a.postForm(ctx, a.tokenEndpoint(), form, &tr); err != nil {
		return Token{}, err
	}
	if tr.Error != "" {
		return Token{}, fmt.Errorf("%w: %s", ErrToken, tr.Error)
	}
	t := tokenFrom(tr)
	if t.RefreshToken == "" {
		t.RefreshToken = refreshToken
	}
	return t, nil
}

// WebAuthCodeURL builds the authorization-code consent URL for a server-side web
// flow. offline_access is among the default scopes, so the exchange returns a
// refresh token. state carries the caller's anti-forgery and routing value.
func (a *Authenticator) WebAuthCodeURL(redirectURI, state string) string {
	q := url.Values{
		"client_id":     {a.clientID},
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"response_mode": {"query"},
		"scope":         {strings.Join(a.scopes, " ")},
		"state":         {state},
	}
	return a.authorizeEndpoint() + "?" + q.Encode()
}

// ExchangeCode trades a web-flow authorization code for tokens using the
// confidential client secret. redirectURI must match the one used to obtain the code.
func (a *Authenticator) ExchangeCode(ctx context.Context, code, redirectURI string) (Token, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {a.clientID},
		"client_secret": {a.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"scope":         {strings.Join(a.scopes, " ")},
	}
	var tr tokenResponse
	if err := a.postForm(ctx, a.tokenEndpoint(), form, &tr); err != nil {
		return Token{}, err
	}
	if tr.Error != "" {
		return Token{}, fmt.Errorf("%w: %s: %s", ErrToken, tr.Error, tr.ErrorDescription)
	}
	return tokenFrom(tr), nil
}

// Refresh exchanges a refresh token for a fresh access token, including the client
// secret when set. The Slack server uses it to run a command as a linked user.
func (a *Authenticator) Refresh(ctx context.Context, refreshToken string) (Token, error) {
	return a.refresh(ctx, refreshToken)
}

// postForm posts a URL-encoded form and decodes the JSON response into out.
func (a *Authenticator) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// tokenEndpoint is the tenant's OAuth token URL.
func (a *Authenticator) tokenEndpoint() string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/token", a.host, a.tenant)
}

// deviceEndpoint is the tenant's device authorization URL.
func (a *Authenticator) deviceEndpoint() string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/devicecode", a.host, a.tenant)
}

// authorizeEndpoint is the tenant's OAuth authorization URL.
func (a *Authenticator) authorizeEndpoint() string {
	return fmt.Sprintf("%s/%s/oauth2/v2.0/authorize", a.host, a.tenant)
}

// tokenFrom builds a Token from a token response, stamping the expiry.
func tokenFrom(tr tokenResponse) Token {
	return Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Scope:        tr.Scope,
		Expiry:       time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
}
