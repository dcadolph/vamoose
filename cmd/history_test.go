package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/audit"
)

// TestEventsForHold confirms the filter keeps only the named hold's events in order.
func TestEventsForHold(t *testing.T) {
	t.Parallel()
	events := []audit.Event{
		{HoldID: "H1", Action: audit.ActionCreated},
		{HoldID: "H2", Action: audit.ActionCreated},
		{HoldID: "H1", Action: audit.ActionApproved},
	}
	got := eventsForHold(events, "H1")
	if len(got) != 2 || got[0].Action != audit.ActionCreated || got[1].Action != audit.ActionApproved {
		t.Errorf("eventsForHold = %+v, want the two H1 events", got)
	}
}

// TestPrintEvent confirms a history line carries the action, workflow, hold, and actor.
func TestPrintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	at := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	printEvent(&buf, audit.Event{Time: at, Workflow: "pto", HoldID: "H1", Action: audit.ActionApproved, Actor: "boss@x.com"})
	out := buf.String()
	for _, want := range []string{"approved", "pto", "H1", "boss@x.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("printEvent output %q missing %q", out, want)
		}
	}
}

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

// TestRunHistoryJSON confirms the command filters by hold and emits JSON. It isolates
// HOME and redirects stdout, so it is not parallel.
func TestRunHistoryJSON(t *testing.T) {
	isolateConfig(t)
	store, err := auditStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, e := range []audit.Event{
		{Time: time.Now(), HoldID: "H1", Workflow: "pto", Action: audit.ActionCreated},
		{Time: time.Now(), HoldID: "H1", Workflow: "pto", Action: audit.ActionApproved, Actor: "boss@x.com"},
		{Time: time.Now(), HoldID: "H2", Workflow: "pto", Action: audit.ActionCreated},
	} {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	out := captureStdout(t, func() {
		if err := runHistory(ctx, []string{"--hold", "H1", "--json"}); err != nil {
			t.Errorf("runHistory: %v", err)
		}
	})
	var got []audit.Event
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events for H1, want 2", len(got))
	}
	if got[1].Action != audit.ActionApproved || got[1].Actor != "boss@x.com" {
		t.Errorf("second H1 event = %+v, want approved by boss", got[1])
	}
}

// TestRunHistoryEmpty confirms the no-history message when nothing is recorded.
func TestRunHistoryEmpty(t *testing.T) {
	isolateConfig(t)
	out := captureStdout(t, func() {
		if err := runHistory(context.Background(), nil); err != nil {
			t.Errorf("runHistory empty: %v", err)
		}
	})
	if !strings.Contains(out, "No run history") {
		t.Errorf("empty history output = %q, want the no-history message", out)
	}

	// Empty --json emits an array, not null, so tooling can parse it.
	jsonOut := captureStdout(t, func() {
		if err := runHistory(context.Background(), []string{"--json"}); err != nil {
			t.Errorf("runHistory empty json: %v", err)
		}
	})
	if strings.TrimSpace(jsonOut) != "[]" {
		t.Errorf("empty --json = %q, want []", strings.TrimSpace(jsonOut))
	}
}
