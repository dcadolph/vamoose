package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/workflow"
)

// isolateConfig points the user config directory at a temporary location so state,
// watch, and team files do not touch the real environment.
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}

// testWindow is a fixed all-day window for workflow execution tests.
func testWindow() (time.Time, time.Time) {
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 0, 5)
}

// loadTestWorkflow loads a built-in workflow, failing the test on error.
func loadTestWorkflow(t *testing.T, name string) workflow.Workflow {
	t.Helper()
	wf, err := workflow.Loader{}.Load(name)
	if err != nil {
		t.Fatalf("load %q: %v", name, err)
	}
	return wf
}

// TestRunWorkflowPTO confirms the pto workflow creates a hold shown free that
// invites the manager as required and stops at the approval gate.
func TestRunWorkflowPTO(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}}
	wf := loadTestWorkflow(t, "pto")

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true,
	})
	if err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if prov.created == nil {
		t.Fatal("hold was not created")
	}
	if prov.created.ShowAs != calendar.ShowFree {
		t.Errorf("showAs = %q, want free", prov.created.ShowAs)
	}
	if len(prov.created.Attendees) != 1 {
		t.Fatalf("attendees = %d, want 1", len(prov.created.Attendees))
	}
	if a := prov.created.Attendees[0]; a.Person.Email != "boss@x.com" || a.Role != calendar.RoleRequired {
		t.Errorf("manager attendee = %+v, want boss required", a)
	}
	if prov.updated != nil {
		t.Error("notify ran before approval")
	}
	if w, _ := loadWatches(); len(w) != 0 {
		t.Errorf("watches = %d, want 0 without --watch", len(w))
	}
	if s, _ := loadState(); s.LastHold.ID != "created-id" {
		t.Errorf("state hold id = %q, want created-id", s.LastHold.ID)
	}
}

// TestRunWorkflowPTOWatch confirms --watch enqueues the hold for the daemon with
// auto-promote set, since a notify step follows approval.
func TestRunWorkflowPTOWatch(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}}
	wf := loadTestWorkflow(t, "pto")

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true, Watch: true,
	})
	if err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	watches, _ := loadWatches()
	if len(watches) != 1 {
		t.Fatalf("watches = %d, want 1", len(watches))
	}
	if w := watches[0]; w.HoldID != "created-id" || w.Workflow != "pto" || w.Step != 1 {
		t.Errorf("watch = %+v, want created-id pto step 1", w)
	}
	if prov.updated != nil {
		t.Error("notify ran before approval")
	}
}

// TestRunWorkflowNotifyOnly confirms a workflow without approval creates a hold with
// no manager and fans out to the team immediately.
func TestRunWorkflowNotifyOnly(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{team: []calendar.Person{{Email: "peer@x.com"}}}
	wf := loadTestWorkflow(t, "notify-only")

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true,
	})
	if err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if len(prov.created.Attendees) != 0 {
		t.Errorf("created attendees = %d, want 0 without approval", len(prov.created.Attendees))
	}
	if prov.updated == nil {
		t.Fatal("notify did not fan out")
	}
	found := false
	for _, a := range prov.updated.Attendees {
		if a.Person.Email == "peer@x.com" && a.Role == calendar.RoleOptional {
			found = true
		}
	}
	if !found {
		t.Error("peer was not added as an optional attendee")
	}
}

// TestRunWorkflowAway confirms the away workflow creates an out-of-office hold with
// no attendees and no follow-on steps.
func TestRunWorkflowAway(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{}
	wf := loadTestWorkflow(t, "away")

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Out", Start: start, End: end, AllDay: true,
	})
	if err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if prov.created.ShowAs != calendar.ShowOOF {
		t.Errorf("showAs = %q, want oof", prov.created.ShowAs)
	}
	if len(prov.created.Attendees) != 0 {
		t.Errorf("attendees = %d, want 0", len(prov.created.Attendees))
	}
	if prov.updated != nil {
		t.Error("away should not update the hold")
	}
}

// TestRunWorkflowDryRun confirms a dry run creates nothing.
func TestRunWorkflowDryRun(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}}
	wf := loadTestWorkflow(t, "pto")

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if prov.created != nil {
		t.Error("dry run created a hold")
	}
	if s, _ := loadState(); s.LastHold.ID != "" {
		t.Error("dry run wrote state")
	}
}

// TestStartedMessage confirms the created-hold summary reads by action, not by the
// workflow name, and names the approver for an approval workflow.
func TestStartedMessage(t *testing.T) {
	t.Parallel()
	held := calendar.Hold{ID: "e1", Attendees: []calendar.Attendee{
		{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired},
	}}
	tests := []struct {
		Name         string
		Workflow     workflow.Workflow
		Hold         calendar.Hold
		WantContains string
	}{{ // Test 0: pto names the approver.
		Name: "pto", Workflow: loadTestWorkflow(t, "pto"), Hold: held,
		WantContains: "sent to boss@x.com for approval",
	}, { // Test 1: notify-only just reports the hold.
		Name: "notify-only", Workflow: loadTestWorkflow(t, "notify-only"), Hold: held,
		WantContains: "Hold created. Hold id: e1",
	}, { // Test 2: away reads as out of office.
		Name: "away", Workflow: loadTestWorkflow(t, "away"), Hold: calendar.Hold{ID: "e2"},
		WantContains: "Marked out of office",
	}, { // Test 3: an event workflow reads as an event.
		Name:     "event",
		Workflow: workflow.Workflow{Name: "ev", Steps: []workflow.Step{{Verb: workflow.VerbEvent}}},
		Hold:     calendar.Hold{ID: "e3"}, WantContains: "Event created. Id: e3",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := startedMessage(test.Workflow, test.Hold)
			if strings.Contains(got, "pto") || strings.Contains(got, "Started") {
				t.Errorf("%s: message leaks workflow name or 'Started': %q", test.Name, got)
			}
			if !strings.Contains(got, test.WantContains) {
				t.Errorf("%s: message = %q, want it to contain %q", test.Name, got, test.WantContains)
			}
		})
	}
}

// TestRunWorkflowEventNeedsSubject confirms an event workflow without a subject errors.
func TestRunWorkflowEventNeedsSubject(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{}
	wf := workflow.Workflow{Name: "ev", Steps: []workflow.Step{{Verb: workflow.VerbEvent}}}

	err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Start: start, End: end, AllDay: true,
	})
	if err == nil {
		t.Fatal("want error for event workflow without subject")
	}
	if prov.created != nil {
		t.Error("hold created despite missing subject")
	}
}

// TestRunStepsWhenGuard confirms a step whose when guard denies is skipped and one
// whose guard allows runs. It gates on attendee count so the result is deterministic
// without a clock.
func TestRunStepsWhenGuard(t *testing.T) {
	isolateConfig(t)
	wf := workflow.Workflow{Name: "guarded", Steps: []workflow.Step{
		{ID: "hold", Verb: workflow.VerbHold},
		{ID: "notify", Verb: workflow.VerbNotify, When: workflow.When{MinAttendees: 2}, Next: "end"},
	}}
	boss := calendar.Attendee{Person: calendar.Person{Email: "boss@x.com"}, Role: calendar.RoleRequired}
	other := calendar.Attendee{Person: calendar.Person{Email: "other@x.com"}, Role: calendar.RoleRequired}

	// One attendee: the guard denies, so notify is skipped.
	small := &mockProvider{team: []calendar.Person{{Email: "peer@x.com"}}}
	held := calendar.Hold{ID: "h1", Attendees: []calendar.Attendee{boss}}
	if err := runSteps(context.Background(), small, "graph", wf, wf.Next(0, ""), held, false); err != nil {
		t.Fatalf("runSteps small: %v", err)
	}
	if small.updated != nil {
		t.Error("notify ran despite the guard denying")
	}

	// Two attendees: the guard allows, so notify runs.
	big := &mockProvider{team: []calendar.Person{{Email: "peer@x.com"}}}
	full := calendar.Hold{ID: "h2", Attendees: []calendar.Attendee{boss, other}}
	if err := runSteps(context.Background(), big, "graph", wf, wf.Next(0, ""), full, false); err != nil {
		t.Fatalf("runSteps big: %v", err)
	}
	if big.updated == nil {
		t.Error("notify was skipped despite the guard allowing")
	}
}

// TestWhenSummary confirms the dry-run guard summary renders each condition and stays
// empty for an unset guard.
func TestWhenSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		When workflow.When
		Want string
	}{{ // Test 0: An unset guard renders nothing.
		When: workflow.When{}, Want: "",
	}, { // Test 1: A day-of-week guard.
		When: workflow.When{DayOfWeek: "mon-fri"}, Want: " when mon-fri",
	}, { // Test 2: A minimum attendee bound.
		When: workflow.When{MinAttendees: 3}, Want: " when 3+ attendees",
	}, { // Test 3: A maximum attendee bound.
		When: workflow.When{MaxAttendees: 4}, Want: " when up to 4 attendees",
	}, { // Test 4: A range and a day set combine.
		When: workflow.When{DayOfWeek: "sat,sun", MinAttendees: 2, MaxAttendees: 5},
		Want: " when sat,sun, 2-5 attendees",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := whenSummary(test.When); got != test.Want {
				t.Errorf("whenSummary = %q, want %q", got, test.Want)
			}
		})
	}
}
