package comms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWebhookNotifier confirms the notifier posts a {"text": ...} JSON body with the
// auth header. The server validates the request and answers 200 only when both are
// correct, so the test asserts on the returned error without sharing state across
// goroutines.
func TestWebhookNotifier(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["text"] != "hello team" {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "bad content type", http.StatusUnsupportedMediaType)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier("Bearer secret")
	if err := n.Notify(context.Background(), srv.URL, "hello team"); err != nil {
		t.Fatalf("Notify with correct body and auth should succeed: %v", err)
	}
}

// TestWebhookNotifierError confirms a non-2xx response surfaces as an error.
func TestWebhookNotifierError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	n := NewWebhookNotifier("")
	if err := n.Notify(context.Background(), srv.URL, "x"); err == nil {
		t.Error("want an error on a non-2xx response")
	}
}
