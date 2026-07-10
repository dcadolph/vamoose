package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// mockProvider is a calendar.Provider whose responses are set per test.
type mockProvider struct {
	// hold is returned by GetHold.
	hold calendar.Hold
	// getErr is returned by GetHold when set.
	getErr error
	// team is returned by Team for the promote path.
	team []calendar.Person
	// mgr is returned by Manager.
	mgr calendar.Person
	// mgrErr is returned by Manager when set.
	mgrErr error
	// created records the last hold passed to CreateHold.
	created *calendar.Hold
	// updated records the last hold passed to UpdateHold.
	updated *calendar.Hold
	// deleted records the last id passed to DeleteHold.
	deleted string
}

func (m *mockProvider) Me(context.Context) (calendar.Person, error) { return calendar.Person{}, nil }
func (m *mockProvider) Manager(context.Context) (calendar.Person, error) {
	return m.mgr, m.mgrErr
}
func (m *mockProvider) Team(context.Context) ([]calendar.Person, error) { return m.team, nil }

func (m *mockProvider) CreateHold(_ context.Context, h calendar.Hold) (calendar.Hold, error) {
	if h.ID == "" {
		h.ID = "created-id"
	}
	m.created = &h
	return h, nil
}

func (m *mockProvider) GetHold(context.Context, string) (calendar.Hold, error) {
	return m.hold, m.getErr
}

func (m *mockProvider) UpdateHold(_ context.Context, h calendar.Hold) (calendar.Hold, error) {
	m.updated = &h
	return h, nil
}

func (m *mockProvider) DeleteHold(_ context.Context, id string) error {
	m.deleted = id
	return nil
}

// managerHold builds a hold whose required attendee carries the given response.
func managerHold(resp calendar.Response) calendar.Hold {
	return calendar.Hold{Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "mgr@x.com"}, Role: calendar.RoleRequired, Response: resp},
	}}
}

// TestAdvanceRun drives the daemon's per-run advance: it loads the workflow, reads
// the manager response, and on approval runs the steps past the gate. It isolates
// HOME for the promote path, so it does not run in parallel.
func TestAdvanceRun(t *testing.T) {
	isolateConfig(t)
	tests := []struct {
		Name        string
		Response    calendar.Response
		GetErr      error
		Workflow    string
		WantRes     pollResult
		WantPromote bool
	}{{ // Test 0: Approval runs the notify step and promotes the team.
		Name: "approved", Response: calendar.ResponseAccepted, Workflow: "pto",
		WantRes: pollApproved, WantPromote: true,
	}, { // Test 1: A decline stops the run.
		Name: "declined", Response: calendar.ResponseDeclined, Workflow: "pto",
		WantRes: pollDeclined,
	}, { // Test 2: No response keeps waiting.
		Name: "pending", Response: calendar.ResponseNotResponded, Workflow: "pto",
		WantRes: pollPending,
	}, { // Test 3: A fetch error fails.
		Name: "fetch error", Response: calendar.ResponseAccepted, GetErr: errors.New("boom"),
		Workflow: "pto", WantRes: pollFailed,
	}, { // Test 4: An unknown workflow fails.
		Name: "unknown workflow", Response: calendar.ResponseAccepted, Workflow: "ghost",
		WantRes: pollFailed,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			prov := &mockProvider{
				hold:   managerHold(test.Response),
				getErr: test.GetErr,
				team:   []calendar.Person{{Email: "peer@x.com"}},
			}
			item := watchItem{Provider: "graph", HoldID: "id", Workflow: test.Workflow, Step: 1}
			res, _, _ := advanceRun(context.Background(), prov, item)
			if res != test.WantRes {
				t.Errorf("%s: advanceRun = %v, want %v", test.Name, res, test.WantRes)
			}
			if test.WantPromote && prov.updated == nil {
				t.Errorf("%s: expected the team to be promoted", test.Name)
			}
			if !test.WantPromote && prov.updated != nil {
				t.Errorf("%s: unexpected promote", test.Name)
			}
		})
	}
}

// TestAdvanceRunBranching confirms the branching workflow takes the notify path on
// acceptance and the note path on decline.
func TestAdvanceRunBranching(t *testing.T) {
	isolateConfig(t)
	const wf = "pto-notify-on-decline" // built-in: approve gate at step 1

	// Accepted follows the accepted branch: notify the team, no note.
	accepted := &mockProvider{
		hold: managerHold(calendar.ResponseAccepted),
		team: []calendar.Person{{Email: "peer@x.com"}},
	}
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: wf, Step: 1}
	if res, _, _ := advanceRun(context.Background(), accepted, item); res != pollApproved {
		t.Errorf("accepted res = %v, want approved", res)
	}
	if accepted.updated == nil {
		t.Error("accepted branch should notify the team")
	}
	if accepted.created != nil {
		t.Error("accepted branch should not create a note")
	}

	// Declined follows the declined branch: create a note, no team notify.
	declined := &mockProvider{hold: managerHold(calendar.ResponseDeclined)}
	if res, _, _ := advanceRun(context.Background(), declined, item); res != pollDeclined {
		t.Errorf("declined res = %v, want declined", res)
	}
	if declined.created == nil {
		t.Error("declined branch should create a note")
	}
	if declined.updated != nil {
		t.Error("declined branch should not notify the team")
	}
}

// TestAdvanceRunTimeout confirms an approve step's timeout runs the expired branch
// once the deadline passes, and holds otherwise. It isolates HOME to load the
// built-in workflow, so it does not run in parallel.
func TestAdvanceRunTimeout(t *testing.T) {
	isolateConfig(t)
	const wf = "pto-cancel-on-timeout" // approve at step 1, timeout 72h, expired -> cancel

	// Past the timeout with a pending manager: the expired branch cancels the hold.
	pending := managerHold(calendar.ResponseNotResponded)
	pending.ID = "id"
	expired := &mockProvider{hold: pending}
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: wf, Step: 1, CreatedAt: time.Now().Add(-100 * time.Hour)}
	if res, _, err := advanceRun(context.Background(), expired, item); res != pollExpired || err != nil {
		t.Errorf("expired res = %v, err = %v; want pollExpired, nil", res, err)
	}
	if expired.deleted == "" {
		t.Error("expired branch should cancel the hold")
	}

	// Within the timeout: still pending, nothing canceled.
	fresh := &mockProvider{hold: managerHold(calendar.ResponseNotResponded)}
	recent := watchItem{Provider: "graph", HoldID: "id", Workflow: wf, Step: 1, CreatedAt: time.Now().Add(-time.Hour)}
	if res, _, _ := advanceRun(context.Background(), fresh, recent); res != pollPending {
		t.Errorf("recent res = %v, want pollPending", res)
	}
	if fresh.deleted != "" {
		t.Error("within the timeout nothing should be canceled")
	}
}

// TestAdvanceRunChain drives a two-approver chain: the first acceptance invites the
// second approver and keeps watching, and the second acceptance completes the flow and
// notifies the team. It isolates HOME to load a user workflow, so it is not parallel.
func TestAdvanceRunChain(t *testing.T) {
	isolateConfig(t)
	dir, err := workflowsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	wf := `{"name":"two-level","steps":[
		{"id":"hold","verb":"hold"},
		{"id":"a1","verb":"approve","manager":true},
		{"id":"a2","verb":"approve","approver":"dir@x.com"},
		{"id":"notify","verb":"notify","team":"optional","next":"end"}]}`
	if err := os.WriteFile(filepath.Join(dir, "two-level.json"), []byte(wf), 0o600); err != nil {
		t.Fatal(err)
	}

	// Gate 1: the manager accepted; the director is invited and the gate advances.
	hold1 := calendar.Hold{ID: "id", Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseAccepted},
	}}
	prov := &mockProvider{hold: hold1, team: []calendar.Person{{Email: "peer@x.com"}}}
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "two-level", Step: 1, Approver: "boss@x.com"}
	res, updated, err := advanceRun(context.Background(), prov, item)
	if err != nil || res != pollAdvanced {
		t.Fatalf("gate 1 res = %v, err = %v; want pollAdvanced", res, err)
	}
	if updated.Step != 2 || updated.Approver != "dir@x.com" {
		t.Errorf("advanced to step %d approver %q, want step 2 dir@x.com", updated.Step, updated.Approver)
	}
	if prov.updated == nil || !hasRequired(prov.updated.Attendees, "dir@x.com") {
		t.Fatalf("director not invited as required: %+v", prov.updated)
	}
	if hasAttendee(prov.updated.Attendees, "peer@x.com") {
		t.Error("the team was notified before the chain completed")
	}

	// Gate 2: the director accepted; the workflow completes and notifies the team.
	hold2 := calendar.Hold{ID: "id", Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseAccepted},
		{Person: calendar.Person{Email: "dir@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseAccepted},
	}}
	prov2 := &mockProvider{hold: hold2, team: []calendar.Person{{Email: "peer@x.com"}}}
	res2, _, err2 := advanceRun(context.Background(), prov2, updated)
	if err2 != nil || res2 != pollApproved {
		t.Fatalf("gate 2 res = %v, err = %v; want pollApproved", res2, err2)
	}
	if prov2.updated == nil || !hasAttendee(prov2.updated.Attendees, "peer@x.com") {
		t.Error("notify should have promoted the team on completion")
	}
}

// hasRequired reports whether the attendees include a required attendee with the email.
func hasRequired(attendees []calendar.Attendee, email string) bool {
	for _, a := range attendees {
		if a.Person.Email == email && a.Role == calendar.RoleRequired {
			return true
		}
	}
	return false
}

// hasAttendee reports whether the attendees include the email in any role.
func hasAttendee(attendees []calendar.Attendee, email string) bool {
	for _, a := range attendees {
		if a.Person.Email == email {
			return true
		}
	}
	return false
}

// TestWatchPathOverride confirms VAMOOSE_WATCH_FILE overrides the default location,
// which the Slack server uses to give each linked user their own watch file.
func TestWatchPathOverride(t *testing.T) {
	t.Setenv("VAMOOSE_WATCH_FILE", "/tmp/vamoose-custom-watch.json")
	p, err := watchPath()
	if err != nil || p != "/tmp/vamoose-custom-watch.json" {
		t.Errorf("watchPath = %q, %v; want the override", p, err)
	}
}

// TestPollAllPrune confirms a hold whose provider cannot be built is kept by
// default and dropped with prune. It cannot run in parallel because it isolates
// the config directory.
func TestPollAllPrune(t *testing.T) {
	isolateConfig(t)
	item := watchItem{Provider: "no-such-provider", HoldID: "h1", Workflow: "pto", Step: 1, Subject: "vet"}
	if err := saveWatches([]watchItem{item}); err != nil {
		t.Fatalf("save: %v", err)
	}
	logger := log.New(io.Discard, "", 0)
	warned := map[string]bool{}

	// Without prune the unbuildable hold is kept.
	pollAll(context.Background(), logger, false, warned)
	if got, _ := loadWatches(); len(got) != 1 {
		t.Fatalf("kept = %d, want 1 without prune", len(got))
	}

	// With prune the unbuildable hold is dropped.
	pollAll(context.Background(), logger, true, warned)
	if got, _ := loadWatches(); len(got) != 0 {
		t.Errorf("after prune = %d, want 0", len(got))
	}
}

// TestWatchStore exercises the watch list on disk against an isolated HOME.
// It cannot run in parallel because it sets process environment variables.
func TestWatchStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	if w, err := loadWatches(); err != nil || w != nil {
		t.Fatalf("loadWatches on empty = %v, %v; want nil, nil", w, err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e1", Workflow: "pto", Step: 1}); err != nil {
		t.Fatalf("addWatch: %v", err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e2"}); err != nil {
		t.Fatalf("addWatch: %v", err)
	}
	if err := addWatch(watchItem{Provider: "graph", HoldID: "e1", Subject: "replaced"}); err != nil {
		t.Fatalf("addWatch replace: %v", err)
	}
	got, err := loadWatches()
	if err != nil {
		t.Fatalf("loadWatches: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("count = %d, want 2", len(got))
	}
	for _, w := range got {
		if w.HoldID == "e1" && w.Subject != "replaced" {
			t.Errorf("e1 subject = %q, want replaced", w.Subject)
		}
	}
}
