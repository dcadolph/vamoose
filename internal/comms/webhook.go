package comms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// maxWebhookResp caps how much of a response body is read for an error message.
const maxWebhookResp = 1 << 16

// WebhookNotifier posts a message to an incoming webhook URL as a JSON body, so any
// service that accepts an incoming webhook (Microsoft Teams, Google Chat, Mattermost,
// and others) can receive a workflow announcement without a service-specific adapter.
// The destination URL is the message channel, so a workflow can target a different
// webhook per message.
type WebhookNotifier struct {
	// authHeader, when set, is sent as the Authorization header for webhooks that need
	// one. Most incoming webhooks carry their secret in the URL and need none.
	authHeader string
	// httpClient performs the request.
	httpClient *http.Client
}

// NewWebhookNotifier returns a notifier that posts to the URL given as the channel,
// sending authHeader as the Authorization header when it is non-empty.
func NewWebhookNotifier(authHeader string) *WebhookNotifier {
	return &WebhookNotifier{authHeader: authHeader, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// Notify posts text to the webhook URL passed as channel. The body is {"text": <text>},
// the shape the common incoming webhooks accept.
func (n *WebhookNotifier) Notify(ctx context.Context, channel, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, channel, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.authHeader != "" {
		req.Header.Set("Authorization", n.authHeader)
	}
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebhookResp))
		return fmt.Errorf("webhook %s: status %d: %s", channel, resp.StatusCode, string(msg))
	}
	return nil
}
