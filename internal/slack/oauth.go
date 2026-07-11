package slack

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dcadolph/vamoose/internal/secret"
)

// installScopes are the bot scopes vamoose requests during OAuth install. users:read.email
// lets the server resolve an approver's email to a Slack user id so it can verify that a
// click on an approval button comes from the authorized approver.
const installScopes = "commands,chat:write,users:read.email"

// TokenStore persists per-workspace bot tokens keyed by Slack team id.
type TokenStore interface {
	// Save records a workspace's bot token.
	Save(teamID, botToken string) error
	// Get returns a workspace's bot token, or an error when absent.
	Get(teamID string) (string, error)
}

// handleInstall redirects to Slack's OAuth consent screen.
func (s *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if s.clientID == "" {
		http.Error(w, "install not configured", http.StatusServiceUnavailable)
		return
	}
	q := url.Values{
		"client_id":    {s.clientID},
		"scope":        {installScopes},
		"redirect_uri": {s.redirectURI()},
		"state":        {s.states.issue()},
	}
	http.Redirect(w, r, "https://slack.com/oauth/v2/authorize?"+q.Encode(), http.StatusFound)
}

// handleOAuthCallback completes the OAuth install, storing the workspace bot token.
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if !s.states.consume(r.URL.Query().Get("state")) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	teamID, token, err := s.exchangeCode(r.Context(), code)
	if err != nil {
		http.Error(w, "install failed", http.StatusBadGateway)
		return
	}
	if err := s.tokens.Save(teamID, token); err != nil {
		http.Error(w, "could not save install", http.StatusInternalServerError)
		return
	}
	s.mx.installs.Inc()
	s.logger.Info("workspace installed", "team", teamID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("vamoose installed to your workspace. You can close this tab."))
}

// exchangeCode exchanges an OAuth code for a workspace id and bot token.
func (s *Server) exchangeCode(ctx context.Context, code string) (teamID, token string, err error) {
	form := url.Values{
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"code":          {code},
		"redirect_uri":  {s.redirectURI()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.oauthBaseURL+"/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		Team        struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if !out.OK {
		return "", "", fmt.Errorf("slack oauth: %s", out.Error)
	}
	return out.Team.ID, out.AccessToken, nil
}

// redirectURI is the OAuth callback URL derived from the public URL.
func (s *Server) redirectURI() string {
	return strings.TrimRight(s.publicURL, "/") + "/slack/oauth/callback"
}

// stateStore holds short-lived OAuth state tokens for CSRF protection.
type stateStore struct {
	mu     sync.Mutex
	states map[string]time.Time
	now    func() time.Time
	ttl    time.Duration
}

// newStateStore returns a state store using the given clock.
func newStateStore(now func() time.Time) *stateStore {
	return &stateStore{states: make(map[string]time.Time), now: now, ttl: 10 * time.Minute}
}

// issue returns a new random state token that expires after the ttl. It returns the
// empty string if the system entropy source fails, which fails the OAuth flow closed:
// an empty state never validates on the callback.
func (s *stateStore) issue() string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[tok] = s.now().Add(s.ttl)
	return tok
}

// consume validates and removes a state token, reporting whether it was valid.
func (s *stateStore) consume(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.states[tok]
	if !ok {
		return false
	}
	delete(s.states, tok)
	return s.now().Before(exp)
}

// FileStore persists workspace bot tokens as a JSON map at a path, sealed with
// AES-256-GCM when an encryption box is set.
type FileStore struct {
	// path is the JSON file location.
	path string
	// box seals and opens the file when set, for a headless host; nil keeps plaintext.
	box *secret.Box
	// mu guards concurrent reads and writes.
	mu sync.Mutex
}

// NewFileStore returns a plaintext token store backed by the file at path.
func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

// NewTokenStore returns a token store that encrypts bot tokens at rest when
// VAMOOSE_SECRET_KEY is set, for a hosted server, otherwise a plaintext 0600 file.
func NewTokenStore(path string) (*FileStore, error) {
	box, err := secret.FromEnv(os.Getenv)
	if errors.Is(err, secret.ErrNoKey) {
		return &FileStore{path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	return &FileStore{path: path, box: box}, nil
}

// Save records a workspace's bot token, creating parent directories as needed.
func (f *FileStore) Save(teamID, botToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[teamID] = botToken
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	var b []byte
	if f.box != nil {
		b, err = json.Marshal(m)
	} else {
		b, err = json.MarshalIndent(m, "", "  ")
	}
	if err != nil {
		return err
	}
	if f.box != nil {
		if b, err = f.box.Seal(b); err != nil {
			return err
		}
	}
	return os.WriteFile(f.path, b, 0o600)
}

// Get returns a workspace's bot token, or an error when it is not installed.
func (f *FileStore) Get(teamID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return "", err
	}
	if tok, ok := m[teamID]; ok {
		return tok, nil
	}
	return "", fmt.Errorf("workspace %s not installed", teamID)
}

// load reads the token map, decrypting when a box is set, and returns an empty map when
// the file is absent.
func (f *FileStore) load() (map[string]string, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if f.box != nil {
		b, err = f.box.Open(b)
		if err != nil {
			return nil, fmt.Errorf("decrypt slack tokens: %w", err)
		}
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
