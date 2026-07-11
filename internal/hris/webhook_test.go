package hris

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWebhookFileLeave confirms the JSON payload, auth header, and content type, and that
// the id is read from the response.
func TestWebhookFileLeave(t *testing.T) {
	t.Parallel()
	var gotBody map[string]string
	var gotAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotCT = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"wh-1"}`)
	}))
	defer srv.Close()

	f := NewWebhookFiler(srv.URL, "Bearer secret")
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	id, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1", TypeID: "77", Start: start, End: end, Note: "beach"})
	if err != nil || id != "wh-1" {
		t.Fatalf("id = %q, err = %v; want wh-1", id, err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth = %q, want Bearer secret", gotAuth)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Errorf("content type = %q, want json", gotCT)
	}
	for k, want := range map[string]string{"employee_id": "E1", "type_id": "77", "start": "2026-08-01", "end": "2026-08-05", "note": "beach"} {
		if gotBody[k] != want {
			t.Errorf("body[%q] = %q, want %q", k, gotBody[k], want)
		}
	}
}

// TestWebhookNoAuth confirms no Authorization header is sent when none is configured.
func TestWebhookNoAuth(t *testing.T) {
	t.Parallel()
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	f := NewWebhookFiler(srv.URL, "")
	if _, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1"}); err != nil {
		t.Fatal(err)
	}
	if hadAuth {
		t.Error("no Authorization header should be sent without configured auth")
	}
}

// TestWebhookError confirms a non-2xx response is an error.
func TestWebhookError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	f := NewWebhookFiler(srv.URL, "")
	if _, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1"}); err == nil {
		t.Error("want an error on a non-2xx response")
	}
}
