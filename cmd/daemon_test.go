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

	"github.com/dcadolph/vamoose/internal/audit"
	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/workflow"
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
	// createErr is returned by CreateHold when set, without recording the hold, to
	// simulate a step failing transiently.
	createErr error
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
	if m.createErr != nil {
		return calendar.Hold{}, m.createErr
	}
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

// TestAdvanceRunWait drives a wait step: it holds until the delay passes, then advances
// to the following approve gate without re-inviting the already-invited manager, and
// completes once the manager approves. It isolates HOME to load a user workflow.
func TestAdvanceRunWait(t *testing.T) {
	isolateConfig(t)
	dir, err := workflowsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	wf := `{"name":"wait-approve","steps":[
		{"id":"hold","verb":"hold"},
		{"id":"pause","verb":"wait","for":"48h"},
		{"id":"ok","verb":"approve","manager":true},
		{"id":"notify","verb":"notify","team":"optional","next":"end"}]}`
	if err := os.WriteFile(filepath.Join(dir, "wait-approve.json"), []byte(wf), 0o600); err != nil {
		t.Fatal(err)
	}
	pending := calendar.Hold{ID: "id", Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseNotResponded},
	}}

	// Within the delay: still waiting.
	early := &mockProvider{hold: pending, team: []calendar.Person{{Email: "peer@x.com"}}}
	recent := watchItem{Provider: "graph", HoldID: "id", Workflow: "wait-approve", Step: 1, CreatedAt: time.Now().Add(-time.Hour)}
	if res, _, _ := advanceRun(context.Background(), early, recent); res != pollPending {
		t.Errorf("within the delay res = %v, want pollPending", res)
	}

	// Delay passed: advance to the approve gate, without re-inviting the manager.
	mid := &mockProvider{hold: pending, team: []calendar.Person{{Email: "peer@x.com"}}}
	elapsed := watchItem{Provider: "graph", HoldID: "id", Workflow: "wait-approve", Step: 1, CreatedAt: time.Now().Add(-100 * time.Hour)}
	res, updated, err := advanceRun(context.Background(), mid, elapsed)
	if err != nil || res != pollAdvanced {
		t.Fatalf("delay passed res = %v, err = %v; want pollAdvanced", res, err)
	}
	if updated.Step != 2 || updated.Approver != "" {
		t.Errorf("advanced to step %d approver %q, want step 2 with no approver", updated.Step, updated.Approver)
	}
	if mid.updated != nil {
		t.Error("advancing to a manager gate should not re-invite anyone")
	}

	// At the approve gate the manager accepts: the workflow completes and notifies.
	accepted := calendar.Hold{ID: "id", Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired, Response: calendar.ResponseAccepted},
	}}
	last := &mockProvider{hold: accepted, team: []calendar.Person{{Email: "peer@x.com"}}}
	if res, _, err := advanceRun(context.Background(), last, updated); err != nil || res != pollApproved {
		t.Fatalf("after approval res = %v, err = %v; want pollApproved", res, err)
	}
	if last.updated == nil {
		t.Error("notify should have run after the manager approved")
	}
}

// TestAdvanceRunResumesAfterTransientFailure drives the checkpoint: an accepted branch
// runs notify, then its note step fails transiently. The daemon records where it stopped,
// and the next poll resumes at the note without repeating the notify. It isolates HOME to
// load a user workflow, so it is not parallel.
func TestAdvanceRunResumesAfterTransientFailure(t *testing.T) {
	isolateConfig(t)
	dir, err := workflowsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The accepted branch notifies the team, then notes it: two side effects, the note
	// second, so a note failure must not re-run the notify on retry.
	wf := `{"name":"notify-then-note","steps":[
		{"id":"hold","verb":"hold"},
		{"id":"gate","verb":"approve","manager":true,"on":{"accepted":"tell","declined":"end"}},
		{"id":"tell","verb":"notify","team":"optional"},
		{"id":"mark","verb":"note","subject":"Logged","next":"end"}]}`
	if err := os.WriteFile(filepath.Join(dir, "notify-then-note.json"), []byte(wf), 0o600); err != nil {
		t.Fatal(err)
	}
	const noteStep = 3

	accepted := managerHold(calendar.ResponseAccepted)
	accepted.ID = "id"
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "notify-then-note", Step: 1}

	// First poll: notify runs, the note fails, and the branch checkpoints at the note.
	flaky := &mockProvider{
		hold:      accepted,
		team:      []calendar.Person{{Email: "peer@x.com"}},
		createErr: errors.New("calendar busy"),
	}
	res, ckpt, err := advanceRun(context.Background(), flaky, item)
	if res != pollFailed || err == nil {
		t.Fatalf("first poll res = %v, err = %v; want pollFailed and an error", res, err)
	}
	if flaky.updated == nil {
		t.Fatal("notify should have run on the first poll")
	}
	if flaky.created != nil {
		t.Fatal("the failed note should not have been recorded")
	}
	if ckpt.Resume != noteStep {
		t.Fatalf("checkpoint Resume = %d, want %d (the note step)", ckpt.Resume, noteStep)
	}
	if ckpt.Step != 1 {
		t.Errorf("checkpoint Step = %d, want 1 (still the gate)", ckpt.Step)
	}

	// Second poll with the checkpoint: the note succeeds and the notify does not repeat.
	ok := &mockProvider{hold: accepted, team: []calendar.Person{{Email: "peer@x.com"}}}
	res2, done, err2 := advanceRun(context.Background(), ok, ckpt)
	if res2 != pollApproved || err2 != nil {
		t.Fatalf("retry res = %v, err = %v; want pollApproved", res2, err2)
	}
	if ok.updated != nil {
		t.Error("notify repeated on the retry; the checkpoint should have skipped it")
	}
	if ok.created == nil {
		t.Error("the note should have been created on the retry")
	}
	if done.Resume != 0 {
		t.Errorf("checkpoint should clear after completion, got Resume = %d", done.Resume)
	}
}

// TestAdvanceRunResumeIgnoresGateFlip confirms a checkpointed branch runs to completion
// even when the gate response flips between polls: once the team was notified, a late
// decline must neither switch to the declined branch nor re-notify. The declined branch
// cancels the hold, so a wrong switch would delete it, which the test asserts never
// happens. It isolates HOME, so it is not parallel.
func TestAdvanceRunResumeIgnoresGateFlip(t *testing.T) {
	isolateConfig(t)
	dir, err := workflowsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Accepted notifies then notes; declined cancels. The note is the flaky second step.
	wf := `{"name":"flip","steps":[
		{"id":"hold","verb":"hold"},
		{"id":"gate","verb":"approve","manager":true,"on":{"accepted":"tell","declined":"scrap"}},
		{"id":"tell","verb":"notify","team":"optional"},
		{"id":"mark","verb":"note","subject":"Logged","next":"end"},
		{"id":"scrap","verb":"cancel"}]}`
	if err := os.WriteFile(filepath.Join(dir, "flip.json"), []byte(wf), 0o600); err != nil {
		t.Fatal(err)
	}
	const noteStep = 3

	accepted := managerHold(calendar.ResponseAccepted)
	accepted.ID = "id"
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "flip", Step: 1}

	// Poll 1: accepted; notify runs, the note fails, so the branch checkpoints at the note.
	p1 := &mockProvider{
		hold:      accepted,
		team:      []calendar.Person{{Email: "peer@x.com"}},
		createErr: errors.New("calendar busy"),
	}
	_, ckpt, _ := advanceRun(context.Background(), p1, item)
	if ckpt.Resume != noteStep {
		t.Fatalf("checkpoint Resume = %d, want %d (the note step)", ckpt.Resume, noteStep)
	}

	// Poll 2: the response has flipped to declined. The committed accepted branch must
	// still finish: the note runs, the notify does not repeat, and the hold is not
	// canceled by the declined branch.
	flip := &mockProvider{
		hold: managerHold(calendar.ResponseDeclined),
		team: []calendar.Person{{Email: "peer@x.com"}},
	}
	res, done, err := advanceRun(context.Background(), flip, ckpt)
	if err != nil {
		t.Fatalf("resume after flip errored: %v", err)
	}
	if res != pollApproved {
		t.Errorf("resume res = %v, want pollApproved (the committed branch completes)", res)
	}
	if flip.created == nil {
		t.Error("the note should have completed on resume")
	}
	if flip.updated != nil {
		t.Error("the notify should not repeat on resume")
	}
	if flip.deleted != "" {
		t.Error("the declined branch must not run: the hold was canceled")
	}
	if done.Resume != 0 {
		t.Errorf("checkpoint should clear after completion, got Resume = %d", done.Resume)
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

// TestAdvanceRunRecordsAudit confirms the daemon writes run-history events: the approval
// naming the approver, and the team notification. It isolates HOME so the audit file is
// the test's own.
func TestAdvanceRunRecordsAudit(t *testing.T) {
	isolateConfig(t)
	prov := &mockProvider{
		hold: managerHold(calendar.ResponseAccepted),
		team: []calendar.Person{{Email: "peer@x.com"}},
	}
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "pto", Step: 1}
	if res, _, _ := advanceRun(context.Background(), prov, item); res != pollApproved {
		t.Fatalf("res = %v, want pollApproved", res)
	}
	store, err := auditStore()
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.Events()
	if err != nil {
		t.Fatal(err)
	}
	var sawApproved, sawNotified bool
	for _, e := range events {
		if e.Action == audit.ActionApproved && e.Actor == "mgr@x.com" {
			sawApproved = true
		}
		if e.Action == audit.ActionNotified {
			sawNotified = true
		}
	}
	if !sawApproved || !sawNotified {
		t.Errorf("audit events = %+v; want approved by mgr@x.com and notified", events)
	}
}

// TestWalkStepsCheckpoints confirms the executor records branch progress after each
// committed step: the next step to run, then a completion marker (-1) at the end.
func TestWalkStepsCheckpoints(t *testing.T) {
	isolateConfig(t)
	wf := workflow.Workflow{Name: "cp", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbNotify, Team: calendar.RoleOptional},
		{Verb: workflow.VerbNote, Subject: "logged", Next: "end"},
	}}
	var cps []int
	prov := &mockProvider{team: []calendar.Person{{Email: "peer@x.com"}}}
	deps := stepDeps{checkpoint: func(r int) { cps = append(cps, r) }}
	gate, err := walkSteps(context.Background(), prov, deps, "graph", wf, 1, calendar.Hold{ID: "h1"})
	if err != nil || gate != -1 {
		t.Fatalf("walk = %d, %v; want -1, nil", gate, err)
	}
	// notify at step 1 checkpoints its next (step 2); note at step 2 checkpoints -1 (end).
	if len(cps) != 2 || cps[0] != 2 || cps[1] != -1 {
		t.Errorf("checkpoints = %v, want [2 -1]", cps)
	}
}

// TestAdvanceRunResumesFromCheckpoint confirms that after a crash mid-branch (modeled by
// a persisted Resume), the daemon resumes at the checkpoint without replaying the steps
// that already ran. It isolates HOME to load a user workflow, so it is not parallel.
func TestAdvanceRunResumesFromCheckpoint(t *testing.T) {
	isolateConfig(t)
	dir, err := workflowsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	wf := `{"name":"cp","steps":[
		{"id":"hold","verb":"hold"},
		{"id":"gate","verb":"approve","manager":true,"on":{"accepted":"tell","declined":"end"}},
		{"id":"tell","verb":"notify","team":"optional"},
		{"id":"mark","verb":"note","subject":"logged","next":"end"}]}`
	if err := os.WriteFile(filepath.Join(dir, "cp.json"), []byte(wf), 0o600); err != nil {
		t.Fatal(err)
	}
	accepted := managerHold(calendar.ResponseAccepted)
	accepted.ID = "id"
	// The prior poll's checkpoint persisted Resume=3 (the note) after notify ran.
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "cp", Step: 1, Resume: 3}
	prov := &mockProvider{hold: accepted, team: []calendar.Person{{Email: "peer@x.com"}}}
	res, _, err := advanceRun(context.Background(), prov, item)
	if res != pollApproved || err != nil {
		t.Fatalf("res = %v, err = %v; want pollApproved", res, err)
	}
	if prov.updated != nil {
		t.Error("notify replayed on resume; the checkpoint should have skipped it")
	}
	if prov.created == nil {
		t.Error("the note (the resume step) did not run")
	}
}

// TestAdvanceRunDropsDone confirms a hold marked done is dropped without fetching it or
// replaying its branch, so a crash before the drop cannot repeat side effects.
func TestAdvanceRunDropsDone(t *testing.T) {
	isolateConfig(t)
	prov := &mockProvider{getErr: errors.New("GetHold should not be called for a done item")}
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "pto", Step: 1, Done: true}
	res, _, err := advanceRun(context.Background(), prov, item)
	if res != pollApproved || err != nil {
		t.Fatalf("done item res = %v, err = %v; want pollApproved, nil", res, err)
	}
}

// TestAdvanceRunCheckpointsToCompletion confirms an accepted run drives the checkpoint to
// a done marker in the watch store, and that re-polling that done item, as would happen on
// a crash before the drop, drops it without replaying the notify.
func TestAdvanceRunCheckpointsToCompletion(t *testing.T) {
	isolateConfig(t)
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "pto", Step: 1}
	if err := addWatch(item); err != nil {
		t.Fatal(err)
	}
	prov := &mockProvider{hold: managerHold(calendar.ResponseAccepted), team: []calendar.Person{{Email: "peer@x.com"}}}
	if res, _, _ := advanceRun(context.Background(), prov, item); res != pollApproved {
		t.Fatalf("completion res = %v, want pollApproved", res)
	}
	got, err := loadWatches()
	if err != nil || len(got) != 1 || !got[0].Done {
		t.Fatalf("watch after completion = %+v, %v; want one done item", got, err)
	}

	// Recovery: re-poll the done item; it drops without touching the calendar again.
	recover := &mockProvider{hold: managerHold(calendar.ResponseAccepted), team: []calendar.Person{{Email: "peer@x.com"}}}
	if res, _, _ := advanceRun(context.Background(), recover, got[0]); res != pollApproved {
		t.Errorf("recovery res = %v, want pollApproved", res)
	}
	if recover.updated != nil {
		t.Error("recovery replayed the notify on a done item")
	}
}

// TestCheckpointForPersists confirms the checkpoint writes the resume cursor and the done
// marker to the watch store immediately.
func TestCheckpointForPersists(t *testing.T) {
	isolateConfig(t)
	item := watchItem{Provider: "graph", HoldID: "id", Workflow: "cp", Step: 1}
	if err := addWatch(item); err != nil {
		t.Fatal(err)
	}
	cp := checkpointFor(item)

	cp(3)
	if got, _ := loadWatches(); len(got) != 1 || got[0].Resume != 3 || got[0].Done {
		t.Fatalf("after cp(3) = %+v, want Resume 3 not done", got)
	}
	cp(-1)
	if got, _ := loadWatches(); len(got) != 1 || !got[0].Done {
		t.Fatalf("after cp(-1) = %+v, want done", got)
	}
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
