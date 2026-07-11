package hris

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// dateLayout is the day format BambooHR expects.
const dateLayout = "2006-01-02"

// maxRespBody caps how much of a response we read for an error message.
const maxRespBody = 1 << 16

// BambooHRFiler files leave with BambooHR's time-off API.
type BambooHRFiler struct {
	// subdomain is the BambooHR company domain.
	subdomain string
	// apiKey authenticates as the basic-auth user; the password is ignored by BambooHR.
	apiKey string
	// status is the request status to file, such as "requested" or "approved".
	status string
	// httpClient performs the request.
	httpClient *http.Client
	// baseURL is the API root, overridable for tests.
	baseURL string
}

// NewBambooHRFiler returns a filer for the company subdomain and API key. An empty status
// defaults to "requested" so the leave enters BambooHR's own approval flow.
func NewBambooHRFiler(subdomain, apiKey, status string) *BambooHRFiler {
	if status == "" {
		status = "requested"
	}
	return &BambooHRFiler{
		subdomain:  subdomain,
		apiKey:     apiKey,
		status:     status,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    "https://api.bamboohr.com",
	}
}

// WithBaseURL overrides the API root, for tests.
func (f *BambooHRFiler) WithBaseURL(u string) *BambooHRFiler {
	f.baseURL = u
	return f
}

// FileLeave adds a time-off request in BambooHR with a PUT to the employee's time-off
// endpoint, authenticated with the API key as the basic-auth user. It returns the created
// request id from the response body or its Location header.
func (f *BambooHRFiler) FileLeave(ctx context.Context, leave Leave) (string, error) {
	if leave.EmployeeID == "" || leave.TypeID == "" {
		return "", fmt.Errorf("bamboohr: employee id and time-off type id are required")
	}
	body, err := json.Marshal(map[string]string{
		"status":        f.status,
		"start":         leave.Start.Format(dateLayout),
		"end":           leave.End.Format(dateLayout),
		"timeOffTypeId": leave.TypeID,
		"notes":         leave.Note,
	})
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/api/gateway.php/%s/v1/employees/%s/time_off/request/",
		f.baseURL, f.subdomain, leave.EmployeeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+basicAuth(f.apiKey, "x"))

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bamboohr: file leave: status %d: %s", resp.StatusCode, string(respBody))
	}
	return requestID(respBody, resp.Header.Get("Location")), nil
}

// requestID pulls the created request id from the JSON body's id field, which HR systems
// return as a number or a string, falling back to the last path segment of the Location
// header.
func requestID(body []byte, location string) string {
	var out struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(body, &out) == nil {
		if id := strings.Trim(string(out.ID), `"`); id != "" && id != "null" {
			return id
		}
	}
	if location != "" {
		seg := location
		for i := len(seg) - 1; i >= 0; i-- {
			if seg[i] == '/' {
				return seg[i+1:]
			}
		}
		return seg
	}
	return ""
}

// basicAuth returns the base64 of user:pass for HTTP Basic auth.
func basicAuth(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}
