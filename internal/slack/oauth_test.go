package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// memStore is an in-memory TokenStore for tests.
type memStore struct{ m map[string]string }

func (s *memStore) Save(teamID, token string) error { s.m[teamID] = token; return nil }
func (s *memStore) Get(teamID string) (string, error) {
	if v, ok := s.m[teamID]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

func TestExchangeCode(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("code") != "the-code" {
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_code"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"access_token":"xoxb-123","team":{"id":"T123"}}`))
	}))
	defer ts.Close()

	s := NewServer("shh", noopRunner,
		WithOAuth("cid", "csec", "https://x.example", &memStore{m: map[string]string{}}),
		WithOAuthBaseURL(ts.URL))

	team, tok, err := s.exchangeCode(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}
	if team != "T123" || tok != "xoxb-123" {
		t.Errorf("got team=%q token=%q, want T123 / xoxb-123", team, tok)
	}
	if _, _, err := s.exchangeCode(context.Background(), "wrong"); err == nil {
		t.Error("want error for an invalid code")
	}
}

func TestStateStore(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	s := newStateStore(func() time.Time { return now })

	tok := s.issue()
	if !s.consume(tok) {
		t.Error("a fresh state should be valid")
	}
	if s.consume(tok) {
		t.Error("a reused state should be rejected")
	}
	if s.consume("unknown") {
		t.Error("an unknown state should be rejected")
	}

	expiring := s.issue()
	now = now.Add(11 * time.Minute)
	if s.consume(expiring) {
		t.Error("an expired state should be rejected")
	}
}

func TestFileStore(t *testing.T) {
	t.Parallel()
	fs := NewFileStore(filepath.Join(t.TempDir(), "sub", "tokens.json"))

	if _, err := fs.Get("T1"); err == nil {
		t.Error("a missing workspace should error")
	}
	if err := fs.Save("T1", "xoxb-1"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := fs.Save("T2", "xoxb-2"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got, _ := fs.Get("T1"); got != "xoxb-1" {
		t.Errorf("T1 = %q, want xoxb-1", got)
	}
	if got, _ := fs.Get("T2"); got != "xoxb-2" {
		t.Errorf("T2 = %q, want xoxb-2", got)
	}
	if err := fs.Save("T1", "xoxb-1b"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if got, _ := fs.Get("T1"); got != "xoxb-1b" {
		t.Errorf("T1 after overwrite = %q, want xoxb-1b", got)
	}
}
