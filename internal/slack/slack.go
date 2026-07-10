// Package slack serves the vamoose Slack app. It verifies Slack request
// signatures and runs vamoose subcommands on behalf of slash commands, so anything
// the CLI does can be driven from Slack. When a command creates a hold that awaits
// approval, it posts Approve and Decline buttons; clicking one promotes or cancels
// the hold. That makes Slack a backend-independent approval signal, so approval
// works even on backends that cannot report calendar accepts. The runner is
// injected, so the package stays testable without spawning processes.
package slack

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// maxBody caps the request body read, guarding against oversized posts.
	maxBody = 1 << 20
	// actionApprove is the action id of the Approve button.
	actionApprove = "vamoose_approve"
	// actionDecline is the action id of the Decline button.
	actionDecline = "vamoose_decline"
)

// holdIDRe extracts a hold id from vamoose command output.
var holdIDRe = regexp.MustCompile(`(?i)hold id:\s*(\S+)`)

// Runner executes a vamoose subcommand with extra environment and returns its
// combined output. env carries the per-user credentials the server injects so a
// command runs as the invoking Slack user; it is nil in single-tenant mode.
type Runner func(ctx context.Context, args, env []string) (string, error)

// Server serves the vamoose Slack endpoints: slash commands and interactivity.
type Server struct {
	// signingSecret verifies that requests came from Slack.
	signingSecret string
	// run executes a vamoose subcommand.
	run Runner
	// httpClient posts delayed responses back to Slack.
	httpClient *http.Client
	// now supplies the current time, injected for tests.
	now func() time.Time
	// maxSkew is how far a request timestamp may drift, guarding against replay.
	maxSkew time.Duration
	// runTimeout bounds a single subcommand run.
	runTimeout time.Duration
	// clientID and clientSecret are the Slack app OAuth credentials, set to enable
	// the "Add to Slack" install flow.
	clientID     string
	clientSecret string
	// publicURL is the server's public base URL, used to build the OAuth redirect.
	publicURL string
	// tokens stores per-workspace bot tokens from installs.
	tokens TokenStore
	// oauthBaseURL is the Slack API root for OAuth, overridable in tests.
	oauthBaseURL string
	// states holds short-lived OAuth CSRF state tokens.
	states *stateStore
	// links stores each Slack user's linked calendar. When set, the server runs in
	// per-user mode: commands run as the invoking user's own calendar.
	links UserLinkStore
	// linkers maps a provider name to its per-user linker.
	linkers map[string]Linker
	// linkStates holds pending per-user OAuth links across the redirect.
	linkStates *linkStateStore
	// perUserEnv supplies extra per-user environment, such as the watch file, added
	// to a command's credentials in per-user mode.
	perUserEnv func(team, user string) []string
}

// Option configures a Server.
type Option func(*Server)

// WithHTTPClient sets the HTTP client used for delayed responses.
func WithHTTPClient(c *http.Client) Option { return func(s *Server) { s.httpClient = c } }

// WithClock sets the time source, for tests.
func WithClock(now func() time.Time) Option { return func(s *Server) { s.now = now } }

// NewServer returns a Slack Server that runs vamoose subcommands via run. It panics
// on an empty signing secret or a nil runner, signaling developer error.
func NewServer(signingSecret string, run Runner, opts ...Option) *Server {
	if signingSecret == "" {
		panic("slack.NewServer: signing secret required")
	}
	if run == nil {
		panic("slack.NewServer: runner required")
	}
	s := &Server{
		signingSecret: signingSecret,
		run:           run,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		now:           time.Now,
		maxSkew:       5 * time.Minute,
		runTimeout:    60 * time.Second,
		oauthBaseURL:  "https://slack.com/api",
	}
	for _, o := range opts {
		o(s)
	}
	s.states = newStateStore(s.now)
	s.linkStates = newLinkStateStore(s.now)
	return s
}

// WithLinkers turns on per-user mode: each Slack user links their own calendar and
// commands run as that user. links persists the links; each linker handles one
// provider's OAuth or credential flow.
func WithLinkers(links UserLinkStore, linkers ...Linker) Option {
	return func(s *Server) {
		s.links = links
		s.linkers = make(map[string]Linker, len(linkers))
		for _, l := range linkers {
			s.linkers[l.Provider()] = l
		}
	}
}

// WithPerUserEnv sets a hook returning extra environment for a linked user, such as
// their watch file, added alongside their calendar credentials for every command.
func WithPerUserEnv(fn func(team, user string) []string) Option {
	return func(s *Server) { s.perUserEnv = fn }
}

// WithOAuth enables the "Add to Slack" install flow with the app credentials, the
// server's public base URL, and a store for per-workspace bot tokens.
func WithOAuth(clientID, clientSecret, publicURL string, store TokenStore) Option {
	return func(s *Server) {
		s.clientID = clientID
		s.clientSecret = clientSecret
		s.publicURL = publicURL
		s.tokens = store
	}
}

// WithOAuthBaseURL overrides the Slack OAuth API root, for tests.
func WithOAuthBaseURL(u string) Option { return func(s *Server) { s.oauthBaseURL = u } }

// WithPublicURL sets the server's public base URL, used to build the OAuth redirect
// for the per-user link flow. WithOAuth also sets it for the install flow.
func WithPublicURL(u string) Option { return func(s *Server) { s.publicURL = u } }

// Handler returns the HTTP handler serving the Slack endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /slack/commands", s.handleCommand)
	mux.HandleFunc("POST /slack/interactivity", s.handleInteractivity)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	if s.clientID != "" {
		mux.HandleFunc("GET /slack/install", s.handleInstall)
		mux.HandleFunc("GET /slack/oauth/callback", s.handleOAuthCallback)
	}
	if len(s.linkers) > 0 {
		mux.HandleFunc("GET /slack/link/callback", s.handleLinkCallback)
	}
	return mux
}

// handleCommand verifies a slash command, acknowledges it immediately, and runs the
// vamoose subcommand asynchronously, posting the result to the response URL.
func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request) {
	body, err := s.verify(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	text := form.Get("text")
	args, err := tokenize(text)
	if err != nil {
		writeMessage(w, "ephemeral", "Could not parse the command: "+err.Error())
		return
	}
	if len(args) == 0 {
		writeMessage(w, "ephemeral", "Usage: /vamoose <command>, for example /vamoose off next week")
		return
	}
	// In per-user mode, link and unlink are served here before the allowlist, since they
	// are Slack-only commands with no CLI subcommand.
	if s.links != nil {
		switch args[0] {
		case "link":
			s.handleLink(w, form, args)
			return
		case "unlink":
			s.handleUnlink(w, form)
			return
		}
	}
	if !allowedSubcommand(args[0]) {
		writeMessage(w, "ephemeral", "The `"+args[0]+"` command is not available from Slack.")
		return
	}
	// Per-user commands run as the invoking user's linked calendar; single-tenant
	// commands run as the server. Either way the team is passed so an approval can bind
	// the approver.
	if s.links != nil {
		writeMessage(w, "ephemeral", "Running `vamoose "+text+"` ...")
		go s.runAsUser(form.Get("response_url"), form.Get("team_id"), form.Get("user_id"), args)
		return
	}
	writeMessage(w, "ephemeral", "Running `vamoose "+text+"` ...")
	go s.runCommand(form.Get("response_url"), args, nil, form.Get("team_id"), "")
}

// allowedSubcommand reports whether a subcommand may be driven from a Slack slash
// command. Server and host operations (daemon, service, slack, mcp, login) are not
// reachable, and neither are the approval actions (promote, cancel), which run only
// through a verified approval button, so a slash command cannot bypass the approver.
func allowedSubcommand(name string) bool {
	switch name {
	case "off", "away", "event", "request", "run", "workflows", "check", "schedule",
		"team", "calendars", "doctor", "whoami", "help", "version":
		return true
	default:
		return false
	}
}

// runCommand runs a slash subcommand and posts the result. When the output shows a
// hold awaiting approval, it posts Approve and Decline buttons instead of plain text.
func (s *Server) runCommand(responseURL string, args, env []string, ownerTeam, ownerUser string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	out, err := s.run(ctx, args, env)
	if err != nil {
		// Report the error to the invoker but log the raw output server side rather than
		// posting it, so internal detail does not leak.
		log.Printf("slack: command %q failed: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(out))
		s.post(responseURL, map[string]any{
			"response_type": "ephemeral",
			"text":          "Command failed: " + err.Error(),
		})
		return
	}
	if id := holdID(out); id != "" && strings.Contains(strings.ToLower(out), "approval") {
		approver := s.resolveApproverID(ctx, ownerTeam, out)
		// In per-user mode the buttons run as the hold owner, so without a verified
		// approver anyone could approve. Withhold the buttons and fall back to the
		// calendar invite the approver can accept, which the per-user poll advances.
		if ownerUser != "" && approver == "" {
			log.Printf("slack: withholding approval buttons for hold %s: could not resolve the approver (needs the users:read.email scope and the approver in the workspace)", id)
			s.post(responseURL, map[string]any{
				"response_type": "in_channel",
				"text":          firstLine(out) + " The approver can accept the calendar invite to approve.",
			})
			return
		}
		value := s.approvalValue(approvalPayload{H: id, T: ownerTeam, U: ownerUser, A: approver})
		s.post(responseURL, map[string]any{
			"response_type": "in_channel",
			"text":          firstLine(out),
			"blocks":        approvalBlocks(firstLine(out), value),
		})
		return
	}
	s.post(responseURL, map[string]any{"response_type": "ephemeral", "text": codeBlock(out)})
}

// approvalPayload is the data an approval button carries: the hold to act on, the owner
// whose calendar it touches in per-user mode, and the approver allowed to click it.
type approvalPayload struct {
	// H is the hold id the action promotes or cancels.
	H string `json:"h"`
	// T is the owning workspace (team) id, set in per-user mode.
	T string `json:"t,omitempty"`
	// U is the owning user id, set in per-user mode so the action runs as them.
	U string `json:"u,omitempty"`
	// A is the approver's Slack user id, set when it could be resolved, so a click can
	// be rejected unless it comes from the approver.
	A string `json:"a,omitempty"`
}

// approvalValue encodes and signs an approval payload into a button value. The form is
// base64(json).base64(hmac) using the Slack signing secret, so a forged or tampered
// value, including one with the approver stripped, is rejected on decode.
func (s *Server) approvalValue(p approvalPayload) string {
	raw, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return enc + "." + s.signValue(enc)
}

// signValue returns the base64 HMAC-SHA256 of enc keyed by the signing secret.
func (s *Server) signValue(enc string) string {
	mac := hmac.New(sha256.New, []byte(s.signingSecret))
	_, _ = io.WriteString(mac, enc)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// decodeApprovalValue verifies an approval button value's signature and returns its
// payload. ok is false when the value is malformed or the signature does not match, so
// a click on a forged value is dropped.
func (s *Server) decodeApprovalValue(value string) (approvalPayload, bool) {
	enc, sig, found := strings.Cut(value, ".")
	if !found {
		return approvalPayload{}, false
	}
	if !hmac.Equal([]byte(sig), []byte(s.signValue(enc))) {
		return approvalPayload{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return approvalPayload{}, false
	}
	var p approvalPayload
	if err := json.Unmarshal(raw, &p); err != nil || p.H == "" {
		return approvalPayload{}, false
	}
	return p, true
}

// approverRe extracts the approver email from a hold's start message so a click can be
// checked against the person the workflow sent the hold to.
var approverRe = regexp.MustCompile(`(?i)sent to (\S+@\S+) for approval`)

// resolveApproverID pulls the approver email from the command output and resolves it to
// a Slack user id through users.lookupByEmail, so a click can be verified. It returns the
// empty string when there is no approver email, no workspace token, or the lookup fails.
func (s *Server) resolveApproverID(ctx context.Context, team, out string) string {
	if team == "" || s.tokens == nil {
		return ""
	}
	m := approverRe.FindStringSubmatch(out)
	if len(m) != 2 {
		return ""
	}
	botToken, err := s.tokens.Get(team)
	if err != nil {
		return ""
	}
	id, err := s.lookupUserByEmail(ctx, botToken, m[1])
	if err != nil {
		log.Printf("slack: could not resolve approver %s to a Slack user: %v", m[1], err)
		return ""
	}
	return id
}

// lookupUserByEmail resolves a Slack user id from an email via users.lookupByEmail using
// the workspace bot token. It needs the users:read.email scope on the install.
func (s *Server) lookupUserByEmail(ctx context.Context, botToken, email string) (string, error) {
	form := url.Values{"email": {email}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.oauthBaseURL+"/users.lookupByEmail", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+botToken)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !out.OK {
		return "", fmt.Errorf("users.lookupByEmail: %s", out.Error)
	}
	return out.User.ID, nil
}

// handleInteractivity verifies a button interaction and acts on Approve or Decline.
func (s *Server) handleInteractivity(w http.ResponseWriter, r *http.Request) {
	body, err := s.verify(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var payload struct {
		Type        string `json:"type"`
		ResponseURL string `json:"response_url"`
		Actions     []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		View struct {
			CallbackID      string      `json:"callback_id"`
			PrivateMetadata string      `json:"private_metadata"`
			State           modalValues `json:"state"`
		} `json:"view"`
	}
	if err := json.Unmarshal([]byte(form.Get("payload")), &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	if payload.Type == "view_submission" {
		s.handleViewSubmission(w, payload.Team.ID, payload.User.ID, payload.View.CallbackID, payload.View.PrivateMetadata, payload.View.State)
		return
	}
	w.WriteHeader(http.StatusOK)
	if len(payload.Actions) == 0 {
		return
	}
	act := payload.Actions[0]
	go s.runAction(payload.ResponseURL, payload.User.ID, act.ActionID, act.Value)
}

// runAction promotes or cancels a hold in response to a button click and updates the
// original message. It verifies the signed button value and, when the value binds an
// approver, rejects a click from anyone but that approver, so a channel member cannot
// approve or cancel someone else's hold.
func (s *Server) runAction(responseURL, clicker, actionID, value string) {
	p, ok := s.decodeApprovalValue(value)
	if !ok {
		s.post(responseURL, map[string]any{"response_type": "ephemeral", "text": "This approval action is invalid or has expired."})
		return
	}
	if p.A != "" && clicker != p.A {
		s.post(responseURL, map[string]any{"response_type": "ephemeral", "text": "Only <@" + p.A + "> can act on this request."})
		return
	}
	var args []string
	var done, verb string
	switch actionID {
	case actionApprove:
		args, done, verb = []string{"promote", "--id", p.H}, "Approved", "approve"
	case actionDecline:
		args, done, verb = []string{"cancel", "--id", p.H}, "Declined", "decline"
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	// In per-user mode, run the action as the hold's owner so it touches their
	// calendar, not the clicker's.
	var env []string
	if p.U != "" && s.links != nil {
		if e, uerr := s.userEnv(ctx, p.T, p.U); uerr == nil {
			env = e
		}
	}
	out, err := s.run(ctx, args, env)
	text := done + "."
	if err != nil {
		// Log the details server side rather than echoing raw command output to the
		// channel, which could leak internal information.
		log.Printf("slack: %s failed for hold %s: %v: %s", verb, p.H, err, strings.TrimSpace(out))
		text = "Could not " + verb + ": the server logged the details."
	}
	s.post(responseURL, map[string]any{"replace_original": true, "text": text})
}

// verify reads the request body and checks the Slack signature and timestamp,
// returning the raw body on success.
func (s *Server) verify(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return nil, err
	}
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp")
	}
	if drift := s.now().Sub(time.Unix(secs, 0)); drift < -s.maxSkew || drift > s.maxSkew {
		return nil, fmt.Errorf("stale timestamp")
	}
	mac := hmac.New(sha256.New, []byte(s.signingSecret))
	_, _ = io.WriteString(mac, "v0:"+ts+":")
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Slack-Signature"))) {
		return nil, fmt.Errorf("invalid signature")
	}
	return body, nil
}

// post sends a JSON payload to a Slack response URL.
func (s *Server) post(responseURL string, payload any) {
	if responseURL == "" {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, responseURL, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if resp, err := s.httpClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

// approvalBlocks builds a Slack message with Approve and Decline buttons carrying the
// signed approval value.
func approvalBlocks(summary, value string) []any {
	return []any{
		map[string]any{
			"type": "section",
			"text": map[string]any{"type": "mrkdwn", "text": summary},
		},
		map[string]any{
			"type": "actions",
			"elements": []any{
				map[string]any{
					"type":      "button",
					"text":      map[string]any{"type": "plain_text", "text": "Approve"},
					"style":     "primary",
					"action_id": actionApprove,
					"value":     value,
				},
				map[string]any{
					"type":      "button",
					"text":      map[string]any{"type": "plain_text", "text": "Decline"},
					"style":     "danger",
					"action_id": actionDecline,
					"value":     value,
				},
			},
		},
	}
}

// writeMessage writes a Slack message response as JSON.
func writeMessage(w http.ResponseWriter, responseType, text string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"response_type": responseType, "text": text})
}

// codeBlock wraps command output in a Slack code block, or notes empty output.
func codeBlock(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return "_(no output)_"
	}
	return "```\n" + out + "\n```"
}

// holdID returns the hold id printed in command output, or empty.
func holdID(out string) string {
	if m := holdIDRe.FindStringSubmatch(out); len(m) == 2 {
		return m[1]
	}
	return ""
}

// firstLine returns the first non-empty line of s, for a short message summary.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return "vamoose"
}

// tokenize splits slash command text into arguments, honoring single and double
// quotes so multi-word values like a subject survive. It never invokes a shell.
func tokenize(text string) ([]string, error) {
	var (
		args   []string
		cur    strings.Builder
		quote  rune
		hasCur bool
	)
	for _, r := range text {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			hasCur = true
		case r == ' ' || r == '\t' || r == '\n':
			if hasCur {
				args = append(args, cur.String())
				cur.Reset()
				hasCur = false
			}
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if hasCur {
		args = append(args, cur.String())
	}
	return args, nil
}

// userEnv resolves a linked user's calendar into the environment that runs a
// command as that user. It returns ErrNotLinked when the user has no link.
func (s *Server) userEnv(ctx context.Context, team, user string) ([]string, error) {
	link, err := s.links.GetLink(team, user)
	if err != nil {
		return nil, err
	}
	linker, ok := s.linkers[link.Provider]
	if !ok {
		return nil, fmt.Errorf("no linker configured for %s", link.Provider)
	}
	return linker.RunEnv(ctx, link)
}

// runAsUser resolves the invoking user's linked calendar and runs the command as
// that user, or asks them to link first.
func (s *Server) runAsUser(responseURL, team, user string, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	env, err := s.userEnv(ctx, team, user)
	if errors.Is(err, ErrNotLinked) {
		s.post(responseURL, map[string]any{
			"response_type": "ephemeral",
			"text":          "Link a calendar first: `/vamoose link " + s.aProvider() + "`",
		})
		return
	}
	if err != nil {
		s.post(responseURL, map[string]any{"response_type": "ephemeral", "text": "Could not authorize your calendar: " + err.Error()})
		return
	}
	env = append(env, s.extraEnv(team, user)...)
	s.runCommand(responseURL, args, env, team, user)
}

// extraEnv returns any per-user environment beyond credentials, such as the watch
// file, from the WithPerUserEnv hook.
func (s *Server) extraEnv(team, user string) []string {
	if s.perUserEnv == nil {
		return nil
	}
	return s.perUserEnv(team, user)
}

// PollUsers advances every linked user's watched holds once, running the daemon as
// each user with their own credentials and watch file. It is a no-op outside
// per-user mode. The Slack server calls it on an interval so per-user approvals
// auto-advance, which the standalone daemon cannot do without per-user credentials.
func (s *Server) PollUsers(ctx context.Context) {
	if s.links == nil {
		return
	}
	ids, err := s.links.List()
	if err != nil {
		return
	}
	for _, id := range ids {
		env, uerr := s.userEnv(ctx, id.Team, id.User)
		if uerr != nil {
			continue
		}
		env = append(env, s.extraEnv(id.Team, id.User)...)
		_, _ = s.run(ctx, []string{"daemon", "--once"}, env)
	}
}

// handleLink starts linking the invoking user's calendar for the named provider.
func (s *Server) handleLink(w http.ResponseWriter, form url.Values, args []string) {
	if len(args) < 2 {
		writeMessage(w, "ephemeral", "Usage: /vamoose link <"+strings.Join(s.providerNames(), "|")+">")
		return
	}
	provider := args[1]
	linker, ok := s.linkers[provider]
	if !ok {
		writeMessage(w, "ephemeral", "Unknown provider "+provider+". Options: "+strings.Join(s.providerNames(), ", "))
		return
	}
	state := s.linkStates.issue(form.Get("team_id"), form.Get("user_id"), provider)
	if authURL := linker.AuthURL(state, s.linkRedirectURI()); authURL != "" {
		writeMessage(w, "ephemeral", "Link your "+provider+" calendar: "+authURL)
		return
	}
	// No OAuth URL: the provider links by a credential modal, such as iCloud.
	if err := s.openCredentialModal(form.Get("team_id"), form.Get("trigger_id"), provider); err != nil {
		writeMessage(w, "ephemeral", "Could not open the "+provider+" link form: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleUnlink removes the invoking user's linked calendar.
func (s *Server) handleUnlink(w http.ResponseWriter, form url.Values) {
	if err := s.links.DeleteLink(form.Get("team_id"), form.Get("user_id")); err != nil {
		writeMessage(w, "ephemeral", "Could not unlink: "+err.Error())
		return
	}
	writeMessage(w, "ephemeral", "Unlinked your calendar.")
}

// handleLinkCallback completes a per-user OAuth link and stores the credentials.
func (s *Server) handleLinkCallback(w http.ResponseWriter, r *http.Request) {
	st, ok := s.linkStates.consume(r.URL.Query().Get("state"))
	if !ok {
		http.Error(w, "invalid or expired link request", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	linker, ok := s.linkers[st.provider]
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	link, err := linker.Exchange(r.Context(), code, s.linkRedirectURI())
	if err != nil {
		http.Error(w, "link exchange failed", http.StatusBadGateway)
		return
	}
	if err := s.links.SaveLink(st.team, st.user, link); err != nil {
		http.Error(w, "could not save link", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("Your " + st.provider + " calendar is linked. You can close this tab."))
}

// linkRedirectURI is the OAuth callback URL for per-user linking.
func (s *Server) linkRedirectURI() string {
	return strings.TrimRight(s.publicURL, "/") + "/slack/link/callback"
}

// aProvider returns a provider name to suggest when a user has not linked, favoring
// google when it is configured.
func (s *Server) aProvider() string {
	if _, ok := s.linkers["google"]; ok {
		return "google"
	}
	for name := range s.linkers {
		return name
	}
	return "google"
}

// providerNames returns the configured provider names, sorted.
func (s *Server) providerNames() []string {
	names := make([]string, 0, len(s.linkers))
	for name := range s.linkers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// credentialModalCallback is the callback id of the credential-entry modal.
const credentialModalCallback = "vamoose_link_credentials"

// modalValues holds a submitted modal's input values, keyed by block id then action
// id, matching Slack's view.state.values shape.
type modalValues struct {
	// Values is the nested block-to-action input map.
	Values map[string]map[string]struct {
		Value string `json:"value"`
	} `json:"values"`
}

// openCredentialModal opens a Slack modal for a provider that links by credentials
// rather than OAuth, currently iCloud. It needs the workspace bot token from an
// install, so it fails when the app is not installed.
func (s *Server) openCredentialModal(teamID, triggerID, provider string) error {
	if s.tokens == nil {
		return fmt.Errorf("credential linking needs the app installed to this workspace")
	}
	if triggerID == "" {
		return fmt.Errorf("missing trigger id")
	}
	botToken, err := s.tokens.Get(teamID)
	if err != nil {
		return fmt.Errorf("workspace not installed: %w", err)
	}
	view := map[string]any{
		"type":             "modal",
		"callback_id":      credentialModalCallback,
		"private_metadata": provider,
		"title":            map[string]any{"type": "plain_text", "text": "Link " + provider},
		"submit":           map[string]any{"type": "plain_text", "text": "Link"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []any{
			credentialInput("username", "Apple ID email"),
			credentialInput("password", "App-specific password"),
		},
	}
	body, err := json.Marshal(map[string]any{"trigger_id": triggerID, "view": view})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.oauthBaseURL+"/views.open", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+botToken)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("views.open: %s", out.Error)
	}
	return nil
}

// credentialInput builds a plain-text input block for the credential modal.
func credentialInput(blockID, label string) map[string]any {
	return map[string]any{
		"type":     "input",
		"block_id": blockID,
		"label":    map[string]any{"type": "plain_text", "text": label},
		"element":  map[string]any{"type": "plain_text_input", "action_id": "value"},
	}
}

// handleViewSubmission stores the credentials a user submitted through the link
// modal and replies so Slack closes it, or returns a field error when either input
// is empty. The provider comes from the modal's private metadata.
func (s *Server) handleViewSubmission(w http.ResponseWriter, teamID, userID, callbackID, provider string, state modalValues) {
	if callbackID != credentialModalCallback || s.links == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	user := state.Values["username"]["value"].Value
	pass := state.Values["password"]["value"].Value
	if user == "" || pass == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"response_action": "errors",
			"errors":          map[string]string{"username": "Enter your Apple ID and app-specific password."},
		})
		return
	}
	link := UserLink{Provider: provider, ICloudUser: user, ICloudAppPassword: pass}
	if err := s.links.SaveLink(teamID, userID, link); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
