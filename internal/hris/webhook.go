package hris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookFiler files leave by posting a JSON payload to a configured URL, so any HR system
// or automation platform (Zapier, n8n, or a custom endpoint) can receive an approved leave
// without a vamoose-specific integration.
type WebhookFiler struct {
	// url is the endpoint the leave is posted to.
	url string
	// authHeader, when set, is sent as the Authorization header, such as "Bearer xyz".
	authHeader string
	// httpClient performs the request.
	httpClient *http.Client
}

// NewWebhookFiler returns a filer that posts leave to url, sending authHeader as the
// Authorization header when it is non-empty.
func NewWebhookFiler(url, authHeader string) *WebhookFiler {
	return &WebhookFiler{
		url:        url,
		authHeader: authHeader,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// webhookPayload is the JSON body posted for a leave.
type webhookPayload struct {
	// EmployeeID is the person taking leave.
	EmployeeID string `json:"employee_id,omitempty"`
	// TypeID is the time-off type.
	TypeID string `json:"type_id,omitempty"`
	// Start is the first day off, as YYYY-MM-DD.
	Start string `json:"start"`
	// End is the last day off, as YYYY-MM-DD.
	End string `json:"end"`
	// Note is the description.
	Note string `json:"note,omitempty"`
}

// FileLeave posts the leave as JSON to the webhook URL and returns the id from the
// response body when present.
func (f *WebhookFiler) FileLeave(ctx context.Context, leave Leave) (string, error) {
	body, err := json.Marshal(webhookPayload{
		EmployeeID: leave.EmployeeID,
		TypeID:     leave.TypeID,
		Start:      leave.Start.Format(dateLayout),
		End:        leave.End.Format(dateLayout),
		Note:       leave.Note,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if f.authHeader != "" {
		req.Header.Set("Authorization", f.authHeader)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("leave webhook: status %d: %s", resp.StatusCode, string(respBody))
	}
	return requestID(respBody, resp.Header.Get("Location")), nil
}
