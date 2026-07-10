package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file mocks a Slack workspace end to end so the per-user flow can be exercised
// without a real workspace, OAuth apps, or a tunnel. It drives the real Server mux
// over HTTP with real Slack v0 signatures and fakes only the true external edges: the
// Slack Web API (a sink that records views.open and response_url posts), each
// provider's OAuth token exchange (a canned Linker), and the vamoose binary (a runner
// that records the injected arguments and environment instead of touching a calendar).
//
// It proves: signature verification, slash routing, link and unlink interception, the
// OAuth link-state round trip, per-user credential injection, approver-verified approval
// buttons that reject a non-approver and act as the hold owner, the iCloud credential
// modal, and per-user daemon advance. It does NOT prove real Slack's exact payloads
// and signatures, real OAuth consent, real calendar mutation, or Block Kit rendering;
// those still need a live workspace.

// demoLinker is a canned Linker for the mock. When withAuth is false it links by a
// credential modal, like iCloud, so its AuthURL is empty.
type demoLinker struct {
	provider string
	withAuth bool
	env      []string
	link     UserLink
}

func (d demoLinker) Provider() string { return d.provider }
func (d demoLinker) AuthURL(state, _ string) string {
	if !d.withAuth {
		return ""
	}
	return "https://auth.example/authorize?state=" + state
}
func (d demoLinker) Exchange(context.Context, string, string) (UserLink, error) { return d.link, nil }
func (d demoLinker) RunEnv(context.Context, UserLink) ([]string, error)         { return d.env, nil }

// memTokens is an in-memory TokenStore seeded with a workspace bot token so the
// credential modal, which needs an install token, can open.
type memTokens map[string]string

func (m memTokens) Save(team, tok string) error { m[team] = tok; return nil }
func (m memTokens) Get(team string) (string, error) {
	if t, ok := m[team]; ok {
		return t, nil
	}
	return "", fmt.Errorf("no token for %s", team)
}

// capturedPost is one request the mock Slack API received.
type capturedPost struct {
	Path string
	Body string
}

// mockSlackAPI stands up the fake Slack Web API. It records every post and answers
// views.open with ok so the modal open succeeds.
func mockSlackAPI(t *testing.T) (*httptest.Server, <-chan capturedPost) {
	t.Helper()
	ch := make(chan capturedPost, 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- capturedPost{Path: r.URL.Path, Body: string(b)}
		if strings.HasSuffix(r.URL.Path, "/views.open") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		// Resolve the approver email to a fixed Slack user id, so the server can bind and
		// verify the approver on the approval buttons.
		if strings.HasSuffix(r.URL.Path, "/users.lookupByEmail") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"ok":true,"user":{"id":"UBOSS"}}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

// recordedCall is one invocation of the mock vamoose runner.
type recordedCall struct {
	Args []string
	Env  []string
}

// recorder is a Runner that records each call and returns canned output. A creating
// command reports a hold awaiting approval so the server posts approval buttons.
type recorder struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (rec *recorder) run(_ context.Context, args, env []string) (string, error) {
	rec.mu.Lock()
	rec.calls = append(rec.calls, recordedCall{Args: append([]string(nil), args...), Env: append([]string(nil), env...)})
	rec.mu.Unlock()
	if len(args) > 0 {
		switch args[0] {
		case "off", "run", "request":
			return "Hold created and sent to boss@x.com for approval.\nHold id: EVT1", nil
		}
	}
	return "ok", nil
}

// last returns the most recent recorded call.
func (rec *recorder) last() recordedCall {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.calls[len(rec.calls)-1]
}

// envHas reports whether a recorded call carried the given KEY=value pair.
func envHas(c recordedCall, kv string) bool {
	for _, e := range c.Env {
		if e == kv {
			return true
		}
	}
	return false
}

// signedForm posts a Slack-signed form to the mux, as Slack would for a slash command
// or interactivity payload.
func signedForm(t *testing.T, h http.Handler, secret, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(form.Encode())
	ts := fmt.Sprintf("%d", time.Now().Unix())
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sign(secret, ts, body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// waitPost reads one captured post or fails after a short timeout.
func waitPost(t *testing.T, ch <-chan capturedPost) capturedPost {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a Slack API call")
		return capturedPost{}
	}
}

// clickPayload builds a block_actions interactivity payload for a user clicking Approve
// on a button carrying value.
func clickPayload(responseURL, user, value string) string {
	payload := map[string]any{
		"type":         "block_actions",
		"response_url": responseURL,
		"team":         map[string]any{"id": "T1"},
		"user":         map[string]any{"id": user},
		"actions":      []any{map[string]any{"action_id": actionApprove, "value": value}},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// TestPerUserSlackWalkthrough drives the whole per-user Slack story against a mocked
// workspace. Run it narrated with:
//
//	go test -v -run TestPerUserSlackWalkthrough ./internal/slack/
func TestPerUserSlackWalkthrough(t *testing.T) {
	const secret = "test-signing-secret"
	sink, posts := mockSlackAPI(t)
	rec := &recorder{}
	links := NewUserLinkFileStore(filepath.Join(t.TempDir(), "links.json"))
	toks := memTokens{"T1": "xoxb-fake-bot-token"}

	google := demoLinker{
		provider: "google", withAuth: true,
		env:  []string{"VAMOOSE_PROVIDER=google", "VAMOOSE_GOOGLE_ACCESS_TOKEN=ya29-alice"},
		link: UserLink{Provider: "google", RefreshToken: "alice-refresh"},
	}
	icloud := demoLinker{
		provider: "icloud", withAuth: false,
		env:  []string{"VAMOOSE_PROVIDER=icloud", "VAMOOSE_ICLOUD_USERNAME=me@icloud.com", "VAMOOSE_ICLOUD_APP_PASSWORD=abcd-efgh"},
		link: UserLink{Provider: "icloud"},
	}

	s := NewServer(secret, rec.run,
		WithOAuth("client-id", "client-secret", sink.URL, toks),
		WithOAuthBaseURL(sink.URL),
		WithPublicURL(sink.URL),
		WithLinkers(links, google, icloud),
		WithPerUserEnv(func(team, user string) []string {
			return []string{"VAMOOSE_WATCH_FILE=/tmp/vamoose/" + team + "-" + user + ".json"}
		}),
	)
	h := s.Handler()
	slash := func(form url.Values) *httptest.ResponseRecorder {
		return signedForm(t, h, secret, "/slack/commands", form)
	}
	form := func(user, text, trigger string) url.Values {
		f := url.Values{"team_id": {"T1"}, "response_url": {sink.URL + "/response"}, "user_id": {user}, "text": {text}}
		if trigger != "" {
			f.Set("trigger_id", trigger)
		}
		return f
	}

	t.Log("workspace: team T1; users U1 (Alice, requester), U2 (Bob, a non-approver), U3 (Carol); approver boss@x.com -> UBOSS")

	// 1. A forged request is rejected before any work happens.
	t.Log("1. security: a request with a bad signature is rejected")
	bad := httptest.NewRequest(http.MethodPost, "/slack/commands", strings.NewReader("text=off"))
	bad.Header.Set("X-Slack-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	bad.Header.Set("X-Slack-Signature", "v0=deadbeef")
	bw := httptest.NewRecorder()
	h.ServeHTTP(bw, bad)
	if bw.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", bw.Code)
	}
	t.Logf("   -> %d Unauthorized", bw.Code)

	// 2. An unlinked user is told to link first.
	t.Log("2. Alice runs `/vamoose off next week` before linking any calendar")
	ack := slash(form("U1", "off next week", ""))
	if ack.Code != 200 {
		t.Fatalf("ack status = %d", ack.Code)
	}
	reply := waitPost(t, posts)
	if !strings.Contains(reply.Body, "link") {
		t.Fatalf("unlinked reply = %s, want a link prompt", reply.Body)
	}
	t.Logf("   -> Slack shows: %s", jsonText(reply.Body))

	// 3. Alice links her Google calendar; the server returns a consent URL with state.
	t.Log("3. Alice runs `/vamoose link google`")
	linkResp := slash(form("U1", "link google", ""))
	state := stateFromAuthURL(t, linkResp.Body.String())
	t.Logf("   -> Slack shows a consent link carrying CSRF state %s...", short(state))

	// 4. Slack redirects back to the OAuth callback; the link is stored.
	t.Log("4. Alice approves consent; Slack calls back to /slack/link/callback")
	cb := httptest.NewRequest(http.MethodGet, "/slack/link/callback?state="+url.QueryEscape(state)+"&code=auth-code", nil)
	cbw := httptest.NewRecorder()
	h.ServeHTTP(cbw, cb)
	if cbw.Code != 200 {
		t.Fatalf("callback status = %d: %s", cbw.Code, cbw.Body.String())
	}
	if link, err := links.GetLink("T1", "U1"); err != nil || link.Provider != "google" {
		t.Fatalf("Alice's link = %+v, %v; want google", link, err)
	}
	t.Logf("   -> stored: T1/U1 -> google (%s)", cbw.Body.String())

	// 5. Now the same command runs on Alice's calendar. The server resolves the approver
	// (boss@x.com -> UBOSS) and posts approval buttons bound to that approver.
	t.Log("5. Alice runs `/vamoose off next week` again, now linked")
	slash(form("U1", "off next week", ""))
	if lookup := waitPost(t, posts); !strings.HasSuffix(lookup.Path, "/users.lookupByEmail") {
		t.Fatalf("expected an approver lookup, got %s", lookup.Path)
	}
	buttons := waitPost(t, posts)
	ranAs := rec.last()
	if !envHas(ranAs, "VAMOOSE_GOOGLE_ACCESS_TOKEN=ya29-alice") {
		t.Fatalf("command env = %v, want Alice's google token", ranAs.Env)
	}
	value := buttonValue(t, []byte(buttons.Body))
	p, ok := s.decodeApprovalValue(value)
	if !ok || p.H != "EVT1" || p.U != "U1" || p.A != "UBOSS" {
		t.Fatalf("approval value = %+v ok=%v, want hold EVT1 owner U1 approver UBOSS", p, ok)
	}
	t.Logf("   -> ran `vamoose %s` with Alice's injected credentials", strings.Join(ranAs.Args, " "))
	t.Log("   -> posted Approve/Decline buttons bound to approver UBOSS, acting as owner U1")

	// 6. A channel member who is NOT the approver clicks Approve; it is rejected and no
	// action runs.
	t.Log("6. Bob (U2), not the approver, clicks Approve")
	if act := signedForm(t, h, secret, "/slack/interactivity", url.Values{"payload": {clickPayload(sink.URL+"/response", "U2", value)}}); act.Code != 200 {
		t.Fatalf("interactivity status = %d", act.Code)
	}
	if reject := waitPost(t, posts); !strings.Contains(reject.Body, "Only") {
		t.Fatalf("Bob's click should be rejected, got: %s", reject.Body)
	}
	if last := rec.last(); len(last.Args) > 0 && last.Args[0] == "promote" {
		t.Fatal("a non-approver's click ran promote")
	}
	t.Log("   -> Slack told Bob only the approver can act")

	// 6b. The approver (UBOSS) clicks Approve; it runs promote as Alice, the owner.
	t.Log("6b. The approver (UBOSS) clicks Approve")
	if act := signedForm(t, h, secret, "/slack/interactivity", url.Values{"payload": {clickPayload(sink.URL+"/response", "UBOSS", value)}}); act.Code != 200 {
		t.Fatalf("interactivity status = %d", act.Code)
	}
	done := waitPost(t, posts)
	promote := rec.last()
	if len(promote.Args) < 3 || promote.Args[0] != "promote" || promote.Args[2] != "EVT1" {
		t.Fatalf("action args = %v, want promote --id EVT1", promote.Args)
	}
	if !envHas(promote, "VAMOOSE_GOOGLE_ACCESS_TOKEN=ya29-alice") {
		t.Fatalf("action env = %v, want Alice's token though the approver clicked", promote.Env)
	}
	t.Logf("   -> ran `vamoose %s` with ALICE's credentials, approved by UBOSS", strings.Join(promote.Args, " "))
	t.Logf("   -> Slack message updated: %s", jsonText(done.Body))

	// 7. Carol links iCloud, which opens a credential modal instead of an OAuth URL.
	t.Log("7. Carol (U3) runs `/vamoose link icloud`, which opens a credential modal")
	slash(form("U3", "link icloud", "trigger-xyz"))
	modal := waitPost(t, posts)
	if !strings.HasSuffix(modal.Path, "/views.open") || !strings.Contains(modal.Body, "App-specific password") {
		t.Fatalf("views.open call = %s %s, want the credential modal", modal.Path, modal.Body)
	}
	t.Logf("   -> server called Slack %s to open a modal titled %q", modal.Path, "Link icloud")

	// 8. Carol submits the modal; the iCloud credentials are stored privately.
	t.Log("8. Carol submits her Apple ID and app-specific password")
	view := map[string]any{
		"type": "view_submission",
		"team": map[string]any{"id": "T1"},
		"user": map[string]any{"id": "U3"},
		"view": map[string]any{
			"callback_id":      credentialModalCallback,
			"private_metadata": "icloud",
			"state": map[string]any{"values": map[string]any{
				"username": map[string]any{"value": map[string]any{"value": "me@icloud.com"}},
				"password": map[string]any{"value": map[string]any{"value": "abcd-efgh"}},
			}},
		},
	}
	vj, _ := json.Marshal(view)
	vs := signedForm(t, h, secret, "/slack/interactivity", url.Values{"payload": {string(vj)}})
	if vs.Code != 200 {
		t.Fatalf("view_submission status = %d: %s", vs.Code, vs.Body.String())
	}
	link, err := links.GetLink("T1", "U3")
	if err != nil || link.Provider != "icloud" || link.ICloudUser != "me@icloud.com" {
		t.Fatalf("Carol's link = %+v, %v; want icloud with her Apple ID", link, err)
	}
	t.Logf("   -> stored: T1/U3 -> icloud (%s); the app password never hit a channel", link.ICloudUser)

	// 9. The server's poll loop advances each user's watched holds as that user.
	t.Log("9. the per-user poll loop runs the daemon once per linked user")
	rec.mu.Lock()
	rec.calls = nil
	rec.mu.Unlock()
	s.PollUsers(context.Background())
	rec.mu.Lock()
	daemonRuns := append([]recordedCall(nil), rec.calls...)
	rec.mu.Unlock()
	if len(daemonRuns) != 2 {
		t.Fatalf("poll ran the daemon %d times, want 2 (Alice + Carol)", len(daemonRuns))
	}
	var sawAlice, sawCarol bool
	for _, c := range daemonRuns {
		if len(c.Args) != 2 || c.Args[0] != "daemon" || c.Args[1] != "--once" {
			t.Fatalf("poll ran %v, want [daemon --once]", c.Args)
		}
		if envHas(c, "VAMOOSE_GOOGLE_ACCESS_TOKEN=ya29-alice") && envHas(c, "VAMOOSE_WATCH_FILE=/tmp/vamoose/T1-U1.json") {
			sawAlice = true
		}
		if envHas(c, "VAMOOSE_ICLOUD_USERNAME=me@icloud.com") && envHas(c, "VAMOOSE_WATCH_FILE=/tmp/vamoose/T1-U3.json") {
			sawCarol = true
		}
	}
	if !sawAlice || !sawCarol {
		t.Fatalf("poll did not run the daemon as both users: alice=%v carol=%v", sawAlice, sawCarol)
	}
	t.Log("   -> daemon --once ran with Alice's google creds + her watch file, and Carol's icloud creds + hers")
	t.Log("mock workspace walkthrough complete: link, run-as-user, owner approval, iCloud modal, per-user daemon all exercised")
}

// jsonText pulls the human text out of a Slack message payload for readable logs.
func jsonText(body string) string {
	var m map[string]any
	if json.Unmarshal([]byte(body), &m) == nil {
		if t, ok := m["text"].(string); ok {
			return firstLine(t)
		}
	}
	return firstLine(body)
}

// stateFromAuthURL extracts the CSRF state from the consent link in a link reply.
func stateFromAuthURL(t *testing.T, body string) string {
	t.Helper()
	const marker = "state="
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no consent URL with state in: %s", body)
	}
	state := body[i+len(marker):]
	for j, r := range state {
		if r == '"' || r == ' ' || r == '\\' || r == '\n' {
			return state[:j]
		}
	}
	return state
}

// short truncates a token for logging.
func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
