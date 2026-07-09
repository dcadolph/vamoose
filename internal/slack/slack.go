// Package slack serves the vamoose Slack app. It verifies Slack request
// signatures and runs vamoose subcommands on behalf of slash commands, so anything
// the CLI does can be driven from Slack. The runner is injected, so the package
// stays testable without spawning processes.
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
	"strconv"
	"strings"
	"time"
)

// maxBody caps the request body read, guarding against oversized posts.
const maxBody = 1 << 20

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
	go s.runAndReport(form.Get("response_url"), args)
}

// runAndReport runs the subcommand and posts its output to the Slack response URL.
func (s *Server) runAndReport(responseURL string, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.runTimeout)
	defer cancel()
	out, err := s.run(ctx, args)
	text := codeBlock(out)
	if err != nil {
		text = "Command failed: " + err.Error() + "\n" + codeBlock(out)
	}
	s.postResponse(responseURL, text)
}

// handleInteractivity verifies an interaction payload. Approve and decline handling
// lands in the next slice.
func (s *Server) handleInteractivity(w http.ResponseWriter, r *http.Request) {
	if _, err := s.verify(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
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

// postResponse posts a message to a Slack response URL.
func (s *Server) postResponse(responseURL, text string) {
	if responseURL == "" {
		return
	}
	b, err := json.Marshal(map[string]string{"response_type": "ephemeral", "text": text})
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
