package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/comms"
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
	if err := runSteps(context.Background(), small, stepDeps{}, "graph", wf, wf.Next(0, ""), held, false); err != nil {
		t.Fatalf("runSteps small: %v", err)
	}
	if small.updated != nil {
		t.Error("notify ran despite the guard denying")
	}

	// Two attendees: the guard allows, so notify runs.
	big := &mockProvider{team: []calendar.Person{{Email: "peer@x.com"}}}
	full := calendar.Hold{ID: "h2", Attendees: []calendar.Attendee{boss, other}}
	if err := runSteps(context.Background(), big, stepDeps{}, "graph", wf, wf.Next(0, ""), full, false); err != nil {
		t.Fatalf("runSteps big: %v", err)
	}
	if big.updated == nil {
		t.Error("notify was skipped despite the guard allowing")
	}
}

// TestRunStepsMessage confirms a message step posts the hold subject to its channel
// through the notifier.
func TestRunStepsMessage(t *testing.T) {
	isolateConfig(t)
	wf := workflow.Workflow{Name: "msg", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbMessage, Channel: "#team", Next: "end"},
	}}
	var gotChannel, gotText string
	notifier := comms.NotifierFunc(func(_ context.Context, channel, text string) error {
		gotChannel, gotText = channel, text
		return nil
	})
	prov := &mockProvider{}
	held := calendar.Hold{ID: "h1", Subject: "Out: beach week"}
	if err := runSteps(context.Background(), prov, stepDeps{notifier: notifier}, "graph", wf, wf.Next(0, ""), held, false); err != nil {
		t.Fatalf("runSteps: %v", err)
	}
	if gotChannel != "#team" || gotText != "Out: beach week" {
		t.Errorf("message = %q to %q, want the hold subject to #team", gotText, gotChannel)
	}
}

// TestRunStepsMessageNoNotifier confirms a message step errors when no comms backend
// is configured, rather than silently dropping the message.
func TestRunStepsMessageNoNotifier(t *testing.T) {
	isolateConfig(t)
	wf := workflow.Workflow{Name: "msg", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbMessage, Channel: "#team", Next: "end"},
	}}
	prov := &mockProvider{}
	err := runSteps(context.Background(), prov, stepDeps{}, "graph", wf, wf.Next(0, ""), calendar.Hold{Subject: "x"}, false)
	if err == nil {
		t.Fatal("want an error when no notifier is configured")
	}
}

// TestRunWorkflowMultiApproverFirstGate confirms a chain invites only the first
// approver, stops at the first gate, and records that approver for the daemon.
func TestRunWorkflowMultiApproverFirstGate(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}}
	wf := workflow.Workflow{Name: "two-level", Steps: []workflow.Step{
		{ID: "hold", Verb: workflow.VerbHold},
		{ID: "a1", Verb: workflow.VerbApprove, Manager: true},
		{ID: "a2", Verb: workflow.VerbApprove, Approver: "dir@x.com"},
		{ID: "notify", Verb: workflow.VerbNotify, Team: calendar.RoleOptional, Next: "end"},
	}}
	if err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true, Watch: true,
	}); err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	// Only the manager is invited; the director comes later, once the manager accepts.
	if len(prov.created.Attendees) != 1 || prov.created.Attendees[0].Person.Email != "boss@x.com" {
		t.Fatalf("attendees = %+v, want only boss@x.com", prov.created.Attendees)
	}
	if prov.updated != nil {
		t.Error("notify ran before the chain completed")
	}
	watches, _ := loadWatches()
	if len(watches) != 1 {
		t.Fatalf("watches = %d, want 1", len(watches))
	}
	if w := watches[0]; w.Step != 1 || w.Approver != "boss@x.com" {
		t.Errorf("watch = step %d approver %q, want step 1 boss@x.com", w.Step, w.Approver)
	}
}

// TestRunWorkflowExplicitFirstApprover confirms an explicit approver on the first
// approve is invited instead of the directory manager.
func TestRunWorkflowExplicitFirstApprover(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{mgr: calendar.Person{Email: "boss@x.com"}}
	wf := workflow.Workflow{Name: "explicit", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbApprove, Approver: "lead@x.com"},
		{Verb: workflow.VerbNotify, Team: calendar.RoleOptional},
	}}
	if err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true,
	}); err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if len(prov.created.Attendees) != 1 || prov.created.Attendees[0].Person.Email != "lead@x.com" {
		t.Errorf("attendees = %+v, want lead@x.com (explicit approver)", prov.created.Attendees)
	}
}

// TestRunWorkflowWaitGate confirms a wait step records the run at that step for the
// daemon, with no approver, and does not run the following steps yet.
func TestRunWorkflowWaitGate(t *testing.T) {
	isolateConfig(t)
	start, end := testWindow()
	prov := &mockProvider{}
	wf := workflow.Workflow{Name: "wait-then-notify", Steps: []workflow.Step{
		{Verb: workflow.VerbHold},
		{Verb: workflow.VerbWait, For: "48h"},
		{Verb: workflow.VerbNotify, Team: calendar.RoleOptional},
	}}
	if err := runWorkflowOn(context.Background(), prov, "graph", wf, runOptions{
		Subject: "Off", Start: start, End: end, AllDay: true, Watch: true,
	}); err != nil {
		t.Fatalf("runWorkflowOn: %v", err)
	}
	if prov.updated != nil {
		t.Error("notify ran before the wait elapsed")
	}
	watches, _ := loadWatches()
	if len(watches) != 1 {
		t.Fatalf("watches = %d, want 1", len(watches))
	}
	if w := watches[0]; w.Step != 1 || w.Approver != "" {
		t.Errorf("watch = step %d approver %q, want step 1 with no approver", w.Step, w.Approver)
	}
}

// TestResolveNotifier confirms the notifier always routes webhook channels, and that
// Slack and email channels are enabled by their settings.
func TestResolveNotifier(t *testing.T) {
	t.Run("none still routes webhooks", func(t *testing.T) {
		t.Setenv("VAMOOSE_SLACK_BOT_TOKEN", "")
		t.Setenv("VAMOOSE_SMTP_HOST", "")
		n := resolveNotifier()
		if n == nil {
			t.Fatal("want a notifier so a webhook channel works with no other backend")
		}
		// A Slack channel with no token still errors clearly rather than silently passing.
		if err := n.Notify(context.Background(), "#team", "x"); err == nil {
			t.Error("want an error for a Slack channel when no token is set")
		}
	})
	t.Run("email only", func(t *testing.T) {
		t.Setenv("VAMOOSE_SLACK_BOT_TOKEN", "")
		t.Setenv("VAMOOSE_SMTP_HOST", "smtp.example.com")
		t.Setenv("VAMOOSE_SMTP_FROM", "vamoose@x.com")
		if resolveNotifier() == nil {
			t.Error("want a notifier when SMTP is configured")
		}
	})
	t.Run("slack only", func(t *testing.T) {
		t.Setenv("VAMOOSE_SMTP_HOST", "")
		t.Setenv("VAMOOSE_SLACK_BOT_TOKEN", "xoxb-token")
		if resolveNotifier() == nil {
			t.Error("want a notifier when a Slack token is set")
		}
	})
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
