package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jsonReq builds a loopback POST with a JSON body.
func jsonReq(path, body string) *http.Request {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Host = "127.0.0.1:8787"
	r.Header.Set("Content-Type", "application/json")
	return r
}

// TestAppAction confirms the action handler allowlists the action and requires a hold id.
func TestAppAction(t *testing.T) {
	t.Parallel()
	h := appAction("/bin/true")

	// An action outside the allowlist is refused.
	w := httptest.NewRecorder()
	h(w, jsonReq("/api/action", `{"action":"delete","holdID":"X"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown action code = %d, want 400", w.Code)
	}
	// A missing hold id is refused.
	w = httptest.NewRecorder()
	h(w, jsonReq("/api/action", `{"action":"check"}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing id code = %d, want 400", w.Code)
	}
	// An allowlisted action runs.
	w = httptest.NewRecorder()
	h(w, jsonReq("/api/action", `{"action":"check","holdID":"X","provider":"caldav"}`))
	if w.Code != http.StatusOK {
		t.Errorf("valid action code = %d, want 200", w.Code)
	}
}

// TestLocalOnly confirms only loopback hosts are served, blunting DNS rebinding.
func TestLocalOnly(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Host string
		Want bool
	}{
		{"127.0.0.1:8787", true},
		{"localhost:8787", true},
		{"[::1]:8787", true},
		{"evil.com", false},
		{"192.168.1.5:8787", false},
	}
	for _, test := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = test.Host
		w := httptest.NewRecorder()
		if got := localOnly(w, r); got != test.Want {
			t.Errorf("localOnly(%q) = %v, want %v", test.Host, got, test.Want)
		}
		if !test.Want && w.Code != http.StatusForbidden {
			t.Errorf("localOnly(%q) status = %d, want 403", test.Host, w.Code)
		}
	}
}

// TestLocalOnlyOrigin confirms a loopback request from a cross-origin page is refused, so
// a page on another site cannot drive the local server.
func TestLocalOnlyOrigin(t *testing.T) {
	t.Parallel()
	// Loopback host but a cross-origin Origin is forbidden.
	r := httptest.NewRequest("POST", "/api/run", nil)
	r.Host = "127.0.0.1:8787"
	r.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	if localOnly(w, r) || w.Code != http.StatusForbidden {
		t.Errorf("cross-origin request should be forbidden, got code %d", w.Code)
	}

	// A loopback Origin is allowed.
	r2 := httptest.NewRequest("POST", "/api/run", nil)
	r2.Host = "127.0.0.1:8787"
	r2.Header.Set("Origin", "http://127.0.0.1:8787")
	if !localOnly(httptest.NewRecorder(), r2) {
		t.Error("same-origin loopback request should be allowed")
	}
}

// TestAppJSON confirms the JSON handler serves results, surfaces errors, and blocks
// non-loopback callers.
func TestAppJSON(t *testing.T) {
	t.Parallel()

	// Success serves the value as JSON.
	w := httptest.NewRecorder()
	appJSON(func() (any, error) { return []string{"a", "b"}, nil })(w, localReq())
	if w.Code != http.StatusOK {
		t.Fatalf("ok code = %d", w.Code)
	}
	var got []string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 2 || got[0] != "a" {
		t.Errorf("body = %s, %v", w.Body.String(), err)
	}

	// An error becomes a 500.
	w = httptest.NewRecorder()
	appJSON(func() (any, error) { return nil, errors.New("boom") })(w, localReq())
	if w.Code != http.StatusInternalServerError {
		t.Errorf("error code = %d, want 500", w.Code)
	}

	// A non-loopback caller is forbidden and fn is not run.
	ran := false
	w = httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "evil.com"
	appJSON(func() (any, error) { ran = true; return 1, nil })(w, r)
	if w.Code != http.StatusForbidden || ran {
		t.Errorf("non-local: code = %d, ran = %v; want 403 and not run", w.Code, ran)
	}
}

// localReq returns a loopback-host request.
func localReq() *http.Request {
	r := httptest.NewRequest("GET", "/api/x", nil)
	r.Host = "127.0.0.1:8787"
	return r
}
