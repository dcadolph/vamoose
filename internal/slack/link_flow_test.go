package slack

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLinker is a test Linker with canned responses.
type fakeLinker struct {
	provider string
	env      []string
	link     UserLink
}

func (f fakeLinker) Provider() string { return f.provider }
func (f fakeLinker) AuthURL(state, _ string) string {
	return "https://auth.example/authorize?state=" + state
}
func (f fakeLinker) Exchange(context.Context, string, string) (UserLink, error) { return f.link, nil }
func (f fakeLinker) RunEnv(context.Context, UserLink) ([]string, error)         { return f.env, nil }

// TestHandleLink confirms a link command returns the provider consent URL.
func TestHandleLink(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	s := NewServer("shh", noopRunner, WithLinkers(store, fakeLinker{provider: "google"}))
	w := httptest.NewRecorder()
	s.handleLink(w, url.Values{"team_id": {"T1"}, "user_id": {"U1"}}, []string{"link", "google"})
	if !strings.Contains(w.Body.String(), "auth.example") {
		t.Errorf("link message missing the consent URL: %s", w.Body.String())
	}
}

// TestLinkCallbackStores confirms a valid callback stores the user's link.
func TestLinkCallbackStores(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	fl := fakeLinker{provider: "google", link: UserLink{Provider: "google", RefreshToken: "rt"}}
	s := NewServer("shh", noopRunner, WithLinkers(store, fl))
	state := s.linkStates.issue("T1", "U1", "google")
	r := httptest.NewRequest("GET", "/slack/link/callback?state="+state+"&code=abc", nil)
	w := httptest.NewRecorder()
	s.handleLinkCallback(w, r)
	if w.Code != 200 {
		t.Fatalf("callback status = %d, want 200", w.Code)
	}
	got, err := store.GetLink("T1", "U1")
	if err != nil || got.RefreshToken != "rt" {
		t.Errorf("stored link = %+v, %v; want refresh token rt", got, err)
	}
}

// TestLinkCallbackRejectsBadState confirms an unknown state is rejected.
func TestLinkCallbackRejectsBadState(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	s := NewServer("shh", noopRunner, WithLinkers(store, fakeLinker{provider: "google"}))
	r := httptest.NewRequest("GET", "/slack/link/callback?state=bogus&code=abc", nil)
	w := httptest.NewRecorder()
	s.handleLinkCallback(w, r)
	if w.Code != 400 {
		t.Errorf("bad-state status = %d, want 400", w.Code)
	}
}

// TestRunAsUserInjectsEnv confirms a linked user's command runs with injected env.
func TestRunAsUserInjectsEnv(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	if err := store.SaveLink("T1", "U1", UserLink{Provider: "google", RefreshToken: "rt"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	fl := fakeLinker{provider: "google", env: []string{"VAMOOSE_PROVIDER=google", "VAMOOSE_GOOGLE_ACCESS_TOKEN=at"}}
	var gotEnv []string
	runner := func(_ context.Context, _ []string, env []string) (string, error) {
		gotEnv = env
		return "ok", nil
	}
	srv, ch := captureServer(t)
	s := NewServer("shh", runner, WithLinkers(store, fl))
	s.runAsUser(srv.URL, "T1", "U1", []string{"whoami"})
	<-ch
	if len(gotEnv) != 2 || gotEnv[1] != "VAMOOSE_GOOGLE_ACCESS_TOKEN=at" {
		t.Errorf("injected env = %v, want the google access token", gotEnv)
	}
}

// TestRunAsUserNotLinked confirms an unlinked user is prompted to link.
func TestRunAsUserNotLinked(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	srv, ch := captureServer(t)
	s := NewServer("shh", noopRunner, WithLinkers(store, fakeLinker{provider: "google"}))
	s.runAsUser(srv.URL, "T1", "nobody", []string{"whoami"})
	if body := <-ch; !strings.Contains(string(body), "link") {
		t.Errorf("unlinked prompt missing: %s", body)
	}
}

// icloudValues builds modal state with the given Apple ID and app password.
func icloudValues(user, pass string) modalValues {
	var mv modalValues
	mv.Values = map[string]map[string]struct {
		Value string `json:"value"`
	}{
		"username": {"value": {Value: user}},
		"password": {"value": {Value: pass}},
	}
	return mv
}

// TestViewSubmissionStoresICloud confirms a submitted credential modal stores the
// iCloud link with its credentials.
func TestViewSubmissionStoresICloud(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	s := NewServer("shh", noopRunner, WithLinkers(store, fakeLinker{provider: "icloud"}))
	w := httptest.NewRecorder()
	s.handleViewSubmission(w, "T1", "U1", credentialModalCallback, "icloud", icloudValues("me@icloud.com", "abcd-efgh"))
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	link, err := store.GetLink("T1", "U1")
	if err != nil || link.Provider != "icloud" || link.ICloudUser != "me@icloud.com" || link.ICloudAppPassword != "abcd-efgh" {
		t.Errorf("stored link = %+v, %v", link, err)
	}
}

// TestViewSubmissionMissingFields returns a Slack field error and stores nothing.
func TestViewSubmissionMissingFields(t *testing.T) {
	t.Parallel()
	store := NewUserLinkFileStore(filepath.Join(t.TempDir(), "l.json"))
	s := NewServer("shh", noopRunner, WithLinkers(store, fakeLinker{provider: "icloud"}))
	w := httptest.NewRecorder()
	s.handleViewSubmission(w, "T1", "U1", credentialModalCallback, "icloud", icloudValues("", ""))
	if !strings.Contains(w.Body.String(), "response_action") {
		t.Errorf("want a Slack errors response, got: %s", w.Body.String())
	}
	if _, err := store.GetLink("T1", "U1"); err == nil {
		t.Error("empty submission should not store a link")
	}
}
