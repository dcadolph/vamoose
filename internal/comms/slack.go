package comms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackNotifier posts messages to a Slack channel using a workspace bot token.
type SlackNotifier struct {
	// token is the Slack bot token authorizing chat.postMessage.
	token string
	// baseURL is the Slack API root, overridable in tests.
	baseURL string
	// httpClient issues the API request.
	httpClient *http.Client
}

// SlackOption configures a SlackNotifier.
type SlackOption func(*SlackNotifier)

// WithSlackBaseURL overrides the Slack API root, for tests.
func WithSlackBaseURL(u string) SlackOption { return func(s *SlackNotifier) { s.baseURL = u } }

// WithSlackHTTPClient sets the HTTP client used for requests.
func WithSlackHTTPClient(c *http.Client) SlackOption {
	return func(s *SlackNotifier) { s.httpClient = c }
}

// NewSlackNotifier returns a Slack notifier authorized by the given bot token.
func NewSlackNotifier(token string, opts ...SlackOption) *SlackNotifier {
	s := &SlackNotifier{
		token:      token,
		baseURL:    "https://slack.com/api",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Notify posts text to the channel through the Slack chat.postMessage API. It returns
// an error when the token or channel is rejected, so a workflow surfaces the failure
// rather than silently dropping the message.
func (s *SlackNotifier) Notify(ctx context.Context, channel, text string) error {
	if channel == "" {
		return fmt.Errorf("slack notify: empty channel")
	}
	body, err := json.Marshal(map[string]string{"channel": channel, "text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack notify: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("slack notify: decode response: %w", err)
	}
	if !out.OK {
		msg := out.Error
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("slack notify: %s", msg)
	}
	return nil
}
