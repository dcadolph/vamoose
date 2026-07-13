package hris

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Balance is the remaining time off of one type for a person.
type Balance struct {
	// TypeID is the HR system's time-off type identifier.
	TypeID string
	// Name is the human name of the type, such as "Vacation", when the system reports it.
	Name string
	// Unit is the balance unit, "days" or "hours".
	Unit string
	// Available is how much of the type remains.
	Available float64
}

// BalanceReader reads a person's remaining leave from an HR system, so vamoose can show a
// balance and warn before a request would overdraw it. It is the read side of the leave
// axis, the counterpart to Filer.
type BalanceReader interface {
	// Balance returns the person's remaining time off by type, as of the given date. A
	// zero asOf means today.
	Balance(ctx context.Context, employeeID string, asOf time.Time) ([]Balance, error)
}

// BalanceReaderFunc adapts a plain function to a BalanceReader.
type BalanceReaderFunc func(ctx context.Context, employeeID string, asOf time.Time) ([]Balance, error)

// Balance calls f.
func (f BalanceReaderFunc) Balance(ctx context.Context, employeeID string, asOf time.Time) ([]Balance, error) {
	return f(ctx, employeeID, asOf)
}

// WebhookBalanceReader reads balances from a configured URL, so any HR system or
// automation platform can report balances without a vamoose-specific integration. It
// issues a GET with employee and as_of query parameters and expects a JSON array of
// {"type","name","unit","available"} objects.
type WebhookBalanceReader struct {
	// url is the endpoint queried for balances.
	url string
	// authHeader, when set, is sent as the Authorization header.
	authHeader string
	// httpClient performs the request.
	httpClient *http.Client
}

// NewWebhookBalanceReader returns a reader that queries url, sending authHeader as the
// Authorization header when it is non-empty.
func NewWebhookBalanceReader(url, authHeader string) *WebhookBalanceReader {
	return &WebhookBalanceReader{
		url:        url,
		authHeader: authHeader,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// balanceJSON is the wire shape of one balance from a webhook.
type balanceJSON struct {
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	Available float64 `json:"available"`
}

// Balance queries the webhook and maps its JSON array to balances.
func (r *WebhookBalanceReader) Balance(ctx context.Context, employeeID string, asOf time.Time) ([]Balance, error) {
	u, err := url.Parse(r.url)
	if err != nil {
		return nil, fmt.Errorf("balance webhook: bad url: %w", err)
	}
	q := u.Query()
	q.Set("employee", employeeID)
	if !asOf.IsZero() {
		q.Set("as_of", asOf.Format(dateLayout))
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if r.authHeader != "" {
		req.Header.Set("Authorization", r.authHeader)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("balance webhook: status %d: %s", resp.StatusCode, string(body))
	}
	var raw []balanceJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("balance webhook: parse: %w", err)
	}
	out := make([]Balance, 0, len(raw))
	for _, b := range raw {
		out = append(out, Balance{TypeID: b.Type, Name: b.Name, Unit: b.Unit, Available: b.Available})
	}
	return out, nil
}
