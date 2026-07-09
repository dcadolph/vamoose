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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

// Runner executes a vamoose subcommand and returns its combined output.
type Runner func(ctx context.Context, args []string) (string, error)

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
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Handler returns the HTTP handler serving the Slack endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /slack/commands", s.handleCommand)
	mux.HandleFunc("POST /slack/interactivity", s.handleInteractivity)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
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
	writeMessage(w, "ephemeral", "Running `vamoose "+text+"` ...")
	go s.runCommand(form.Get("response_url"), args)
}

// runCommand runs a slash subcommand and posts the result. When the output shows a
// hold awaiting approval, it posts Approve and Decline buttons instead of plain text.
func (s *Server) runCommand(responseURL string, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	out, err := s.run(ctx, args)
	if err != nil {
		s.post(responseURL, map[string]any{
			"response_type": "ephemeral",
			"text":          "Command failed: " + err.Error() + "\n" + codeBlock(out),
		})
		return
	}
	if id := holdID(out); id != "" && strings.Contains(strings.ToLower(out), "approval") {
		s.post(responseURL, map[string]any{
			"response_type": "in_channel",
			"text":          firstLine(out),
			"blocks":        approvalBlocks(strings.TrimSpace(out), id),
		})
		return
	}
	s.post(responseURL, map[string]any{"response_type": "ephemeral", "text": codeBlock(out)})
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
		ResponseURL string `json:"response_url"`
		Actions     []struct {
			ActionID string `json:"action_id"`
			Value    string `json:"value"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(form.Get("payload")), &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	if len(payload.Actions) == 0 {
		return
	}
	act := payload.Actions[0]
	go s.runAction(payload.ResponseURL, act.ActionID, act.Value)
}

// runAction promotes or cancels a hold in response to a button click and updates the
// original message.
func (s *Server) runAction(responseURL, actionID, holdID string) {
	var args []string
	var verb string
	switch actionID {
	case actionApprove:
		args, verb = []string{"promote", "--id", holdID}, "Approved"
	case actionDecline:
		args, verb = []string{"cancel", "--id", holdID}, "Declined"
	default:
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	out, err := s.run(ctx, args)
	text := verb + ".\n" + codeBlock(out)
	if err != nil {
		text = "Could not " + strings.ToLower(verb) + ": " + err.Error() + "\n" + codeBlock(out)
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

// approvalBlocks builds a Slack message with Approve and Decline buttons carrying
// the hold id.
func approvalBlocks(summary, holdID string) []any {
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
					"value":     holdID,
				},
				map[string]any{
					"type":      "button",
					"text":      map[string]any{"type": "plain_text", "text": "Decline"},
					"style":     "danger",
					"action_id": actionDecline,
					"value":     holdID,
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
