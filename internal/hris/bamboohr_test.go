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

// TestBambooHRFileLeave confirms the request method, path, auth, and body, and that the
// created id is read from the response.
func TestBambooHRFileLeave(t *testing.T) {
	t.Parallel()
	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":123}`)
	}))
	defer srv.Close()

	f := NewBambooHRFiler("acme", "key123", "approved").WithBaseURL(srv.URL)
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	id, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1", TypeID: "78", Start: start, End: end, Note: "beach"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "123" {
		t.Errorf("id = %q, want 123", id)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if !strings.Contains(gotPath, "/acme/v1/employees/E1/time_off/request/") {
		t.Errorf("path = %s, want the employee time-off endpoint", gotPath)
	}
	if want := "Basic " + basicAuth("key123", "x"); gotAuth != want {
		t.Errorf("auth = %s, want %s", gotAuth, want)
	}
	for k, want := range map[string]string{"status": "approved", "start": "2026-08-01", "end": "2026-08-05", "timeOffTypeId": "78", "notes": "beach"} {
		if gotBody[k] != want {
			t.Errorf("body[%q] = %q, want %q", k, gotBody[k], want)
		}
	}
}

// TestBambooHRIDFromLocation confirms the id falls back to the Location header's last
// path segment when the body has none.
func TestBambooHRIDFromLocation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/api/gateway.php/acme/v1/time_off/requests/456")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	f := NewBambooHRFiler("acme", "k", "").WithBaseURL(srv.URL)
	if id, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1", TypeID: "1"}); err != nil || id != "456" {
		t.Errorf("id = %q, err = %v; want 456", id, err)
	}
}

// TestBambooHRError confirms a non-2xx response is an error.
func TestBambooHRError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, "nope")
	}))
	defer srv.Close()
	f := NewBambooHRFiler("acme", "k", "").WithBaseURL(srv.URL)
	if _, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1", TypeID: "1"}); err == nil {
		t.Error("want an error on a non-2xx response")
	}
}

// TestBambooHRRequiresIDs confirms the filer rejects a leave without both ids before any
// HTTP call.
func TestBambooHRRequiresIDs(t *testing.T) {
	t.Parallel()
	f := NewBambooHRFiler("acme", "k", "")
	if _, err := f.FileLeave(context.Background(), Leave{TypeID: "1"}); err == nil {
		t.Error("want an error when the employee id is missing")
	}
	if _, err := f.FileLeave(context.Background(), Leave{EmployeeID: "E1"}); err == nil {
		t.Error("want an error when the type id is missing")
	}
}

// TestFilerFunc confirms the func adapter passes the leave through.
func TestFilerFunc(t *testing.T) {
	t.Parallel()
	var got Leave
	var f Filer = FilerFunc(func(_ context.Context, l Leave) (string, error) { got = l; return "ok", nil })
	id, _ := f.FileLeave(context.Background(), Leave{EmployeeID: "E9"})
	if id != "ok" || got.EmployeeID != "E9" {
		t.Errorf("FilerFunc did not pass through: id=%q got=%+v", id, got)
	}
}
