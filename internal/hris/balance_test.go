package hris

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWebhookBalanceReader confirms the reader sends the employee query and auth header
// and maps the JSON array to balances.
func TestWebhookBalanceReader(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("employee") != "E1" {
			http.Error(w, "bad employee", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[{"type":"19","name":"Vacation","unit":"days","available":12.5}]`))
	}))
	defer srv.Close()

	r := NewWebhookBalanceReader(srv.URL, "Bearer k")
	got, err := r.Balance(context.Background(), "E1", time.Time{})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d balances, want 1", len(got))
	}
	if got[0].Name != "Vacation" || got[0].TypeID != "19" || got[0].Unit != "days" || got[0].Available != 12.5 {
		t.Errorf("balance = %+v, want Vacation 12.5 days type 19", got[0])
	}
}

// TestWebhookBalanceReaderError confirms a non-2xx response surfaces as an error.
func TestWebhookBalanceReaderError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewWebhookBalanceReader(srv.URL, "")
	if _, err := r.Balance(context.Background(), "E1", time.Time{}); err == nil {
		t.Error("want an error on a non-2xx response")
	}
}

// TestBambooHRBalanceReader confirms the reader queries the calculator endpoint and parses
// BambooHR's string balances into numbers.
func TestBambooHRBalanceReader(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/employees/E1/time_off/calculator") {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`[{"timeOffType":"78","name":"Vacation","units":"days","balance":"5.0"}]`))
	}))
	defer srv.Close()

	r := NewBambooHRBalanceReader("acme", "key").WithBaseURL(srv.URL)
	got, err := r.Balance(context.Background(), "E1", time.Time{})
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d balances, want 1", len(got))
	}
	if got[0].TypeID != "78" || got[0].Name != "Vacation" || got[0].Unit != "days" || got[0].Available != 5.0 {
		t.Errorf("balance = %+v, want Vacation 5 days type 78", got[0])
	}
}

// TestBambooHRBalanceReaderRequiresEmployee confirms an empty employee id errors before a
// request is made.
func TestBambooHRBalanceReaderRequiresEmployee(t *testing.T) {
	t.Parallel()
	r := NewBambooHRBalanceReader("acme", "key")
	if _, err := r.Balance(context.Background(), "", time.Time{}); err == nil {
		t.Error("want an error for an empty employee id")
	}
}
