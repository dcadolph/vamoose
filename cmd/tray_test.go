package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/audit"
)

// trayTestServer serves the dashboard endpoints the tray reads, with configurable
// bodies, and returns a client pointed at it.
func trayTestServer(t *testing.T, handlers map[string]http.HandlerFunc) *trayClient {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range handlers {
		mux.HandleFunc(pattern, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &trayClient{base: srv.URL, http: srv.Client()}
}

// TestTrayClientHealth covers the up, down, and unreachable cases.
func TestTrayClientHealth(t *testing.T) {
	t.Parallel()
	up := trayTestServer(t, map[string]http.HandlerFunc{
		"GET /health": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	})
	if !up.Health(context.Background()) {
		t.Error("Health = false for a 200 server")
	}

	down := trayTestServer(t, map[string]http.HandlerFunc{
		"GET /health": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
	})
	if down.Health(context.Background()) {
		t.Error("Health = true for a 503 server")
	}

	unreachable := &trayClient{base: "http://127.0.0.1:1", http: &http.Client{Timeout: 200 * time.Millisecond}}
	if unreachable.Health(context.Background()) {
		t.Error("Health = true for an unreachable server")
	}
}

// TestTrayClientWatches confirms decoding, including the JSON null an empty store returns.
func TestTrayClientWatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Body      string
		WantCount int
		WantHold  string
	}{{ // Test 0: A real watch list decodes.
		Body: `[{"provider":"graph","hold_id":"e1","workflow":"pto","step":1,"subject":"Off"}]`,
		WantCount: 1, WantHold: "e1",
	}, { // Test 1: JSON null decodes to an empty list.
		Body: `null`, WantCount: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			c := trayTestServer(t, map[string]http.HandlerFunc{
				"GET /api/watches": func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte(test.Body))
				},
			})
			got, err := c.Watches(context.Background())
			if err != nil {
				t.Fatalf("Watches: %v", err)
			}
			if len(got) != test.WantCount {
				t.Fatalf("count = %d, want %d", len(got), test.WantCount)
			}
			if test.WantCount > 0 && got[0].HoldID != test.WantHold {
				t.Errorf("hold = %q, want %q", got[0].HoldID, test.WantHold)
			}
		})
	}
}

// TestTrayClientHistory confirms the cap and newest-first ordering.
func TestTrayClientHistory(t *testing.T) {
	t.Parallel()
	c := trayTestServer(t, map[string]http.HandlerFunc{
		"GET /api/history": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`[{"action":"a1"},{"action":"a2"},{"action":"a3"}]`))
		},
	})
	got, err := c.History(context.Background(), 2)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 2 || got[0].Action != "a3" || got[1].Action != "a2" {
		t.Errorf("history = %+v, want newest-first [a3 a2]", got)
	}
}

// TestTrayClientAction covers the accepted and refused cases.
func TestTrayClientAction(t *testing.T) {
	t.Parallel()
	var gotBody string
	c := trayTestServer(t, map[string]http.HandlerFunc{
		"POST /api/action": func(w http.ResponseWriter, r *http.Request) {
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			gotBody = string(b)
			if strings.Contains(gotBody, `"cancel"`) {
				http.Error(w, "refused", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		},
	})
	if err := c.Action(context.Background(), "check", "e1", "graph"); err != nil {
		t.Fatalf("Action check: %v", err)
	}
	for _, want := range []string{`"action":"check"`, `"holdID":"e1"`, `"provider":"graph"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("body %s missing %s", gotBody, want)
		}
	}
	if err := c.Action(context.Background(), "cancel", "e1", "graph"); err == nil {
		t.Error("Action cancel: want an error for a 400 response")
	}
}

// TestTrayClientVersion confirms the version passthrough and the error case.
func TestTrayClientVersion(t *testing.T) {
	t.Parallel()
	c := trayTestServer(t, map[string]http.HandlerFunc{
		"GET /api/version": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("vamoose 9.9.9\n"))
		},
	})
	if got := c.Version(context.Background()); got != "vamoose 9.9.9" {
		t.Errorf("Version = %q, want trimmed vamoose 9.9.9", got)
	}
	dead := &trayClient{base: "http://127.0.0.1:1", http: &http.Client{Timeout: 200 * time.Millisecond}}
	if got := dead.Version(context.Background()); got != "" {
		t.Errorf("Version = %q for an unreachable server, want empty", got)
	}
}

// TestTrayWatchLine covers the subject, hold-id fallback, and approver suffix.
func TestTrayWatchLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In   watchItem
		Want string
	}{{ // Test 0: Subject with an approver.
		In:   watchItem{Subject: "Off Friday", Workflow: "pto", Approver: "boss@x.com"},
		Want: "Off Friday  (pto · awaiting boss@x.com)",
	}, { // Test 1: No subject falls back to the hold id, no approver suffix.
		In:   watchItem{HoldID: "e42", Workflow: "away"},
		Want: "e42  (away)",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := trayWatchLine(test.In); got != test.Want {
				t.Errorf("line = %q, want %q", got, test.Want)
			}
		})
	}
}

// TestTrayEventLine covers the full line and the sparse-event fallbacks.
func TestTrayEventLine(t *testing.T) {
	t.Parallel()
	when := time.Date(2026, 7, 13, 9, 30, 0, 0, time.Local)
	tests := []struct {
		In   audit.Event
		Want string
	}{{ // Test 0: Everything present.
		In:   audit.Event{Action: "approved", Workflow: "pto", Actor: "boss@x.com", Time: when},
		Want: "approved  pto · boss@x.com  2026-07-13 09:30",
	}, { // Test 1: Bare action only.
		In:   audit.Event{Action: "started"},
		Want: "started",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := trayEventLine(test.In); got != test.Want {
				t.Errorf("line = %q, want %q", got, test.Want)
			}
		})
	}
}

// TestTrayServicesEnsure covers the no-spawn short circuits: a healthy server, and a
// spawn already in flight. The real spawn path would re-execute the test binary, so it
// is exercised by the compile-only GUI layer instead.
func TestTrayServicesEnsure(t *testing.T) {
	t.Parallel()
	healthy := trayTestServer(t, map[string]http.HandlerFunc{
		"GET /health": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	})
	s := &trayServices{addr: "127.0.0.1:0"}
	if spawned, err := s.Ensure(context.Background(), healthy); spawned || err != nil {
		t.Errorf("Ensure healthy = %v, %v; want false, nil", spawned, err)
	}

	down := &trayClient{base: "http://127.0.0.1:1", http: &http.Client{Timeout: 200 * time.Millisecond}}
	inflight := &trayServices{addr: "127.0.0.1:0", server: &trayChild{done: make(chan struct{})}}
	if spawned, err := inflight.Ensure(context.Background(), down); spawned || err != nil {
		t.Errorf("Ensure inflight = %v, %v; want false, nil", spawned, err)
	}
}

// TestTrayChildAlive covers the nil receiver and the reaped child.
func TestTrayChildAlive(t *testing.T) {
	t.Parallel()
	var missing *trayChild
	if missing.alive() {
		t.Error("nil child reports alive")
	}
	reaped := &trayChild{done: make(chan struct{})}
	if !reaped.alive() {
		t.Error("running child reports dead")
	}
	close(reaped.done)
	if reaped.alive() {
		t.Error("reaped child reports alive")
	}
	reaped.stop()
}

// TestTitleCase covers the label capitalization and the empty case.
func TestTitleCase(t *testing.T) {
	t.Parallel()
	if got := titleCase("promote"); got != "Promote" {
		t.Errorf("titleCase = %q, want Promote", got)
	}
	if got := titleCase(""); got != "" {
		t.Errorf("titleCase empty = %q, want empty", got)
	}
}
