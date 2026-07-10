package workflow

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name     string
		Workflow Workflow
		Want     error
	}{{ // Test 0: A hold, approve, notify sequence is valid.
		Name: "pto",
		Workflow: Workflow{Name: "pto", Steps: []Step{
			{Verb: VerbHold, ShowAs: calendar.ShowFree},
			{Verb: VerbApprove, Manager: true},
			{Verb: VerbNotify, Team: calendar.RoleOptional},
		}},
		Want: nil,
	}, { // Test 1: A lone away step is valid.
		Name:     "away",
		Workflow: Workflow{Name: "away", Steps: []Step{{Verb: VerbAway, ShowAs: calendar.ShowOOF}}},
		Want:     nil,
	}, { // Test 2: Hold then notify with no approval is valid.
		Name: "notify-only",
		Workflow: Workflow{Name: "notify-only", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbNotify},
		}},
		Want: nil,
	}, { // Test 3: A missing name is invalid.
		Name:     "no name",
		Workflow: Workflow{Steps: []Step{{Verb: VerbAway}}},
		Want:     ErrInvalid,
	}, { // Test 4: No steps is invalid.
		Name:     "no steps",
		Workflow: Workflow{Name: "empty"},
		Want:     ErrInvalid,
	}, { // Test 5: An unknown verb is invalid.
		Name:     "unknown verb",
		Workflow: Workflow{Name: "x", Steps: []Step{{Verb: "teleport"}}},
		Want:     ErrUnknownVerb,
	}, { // Test 6: An invalid showAs is invalid.
		Name:     "bad showAs",
		Workflow: Workflow{Name: "x", Steps: []Step{{Verb: VerbHold, ShowAs: "invisible"}}},
		Want:     ErrInvalid,
	}, { // Test 7: An invalid team role is invalid.
		Name: "bad team role",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbNotify, Team: "bystander"},
		}},
		Want: ErrInvalid,
	}, { // Test 8: No creating step is invalid.
		Name:     "no creator",
		Workflow: Workflow{Name: "x", Steps: []Step{{Verb: VerbNotify}}},
		Want:     ErrInvalid,
	}, { // Test 9: Two creating steps are invalid.
		Name: "two creators",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbAway},
		}},
		Want: ErrInvalid,
	}, { // Test 10: A creating step that is not first is invalid.
		Name: "creator not first",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbApprove}, {Verb: VerbHold},
		}},
		Want: ErrInvalid,
	}, { // Test 11: Notifying before approval is invalid.
		Name: "notify before approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbNotify}, {Verb: VerbApprove},
		}},
		Want: ErrInvalid,
	}, { // Test 12: More than one approve step is invalid.
		Name: "two approvals",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbApprove}, {Verb: VerbApprove}, {Verb: VerbNotify},
		}},
		Want: ErrInvalid,
	}, { // Test 13: Approval after a non-hold creator is invalid.
		Name: "event then approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbEvent}, {Verb: VerbApprove},
		}},
		Want: ErrInvalid,
	}, { // Test 14: Approval after away is invalid.
		Name: "away then approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbAway}, {Verb: VerbApprove},
		}},
		Want: ErrInvalid,
	}, { // Test 15: A cancel after approval is valid now the walker runs it.
		Name: "cancel after approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbApprove}, {Verb: VerbCancel},
		}},
		Want: nil,
	}, { // Test 16: Away then notify with no approval is valid.
		Name: "away then notify",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbAway}, {Verb: VerbNotify},
		}},
		Want: nil,
	}, { // Test 17: A branching pto with accept and decline paths is valid.
		Name: "branching pto",
		Workflow: Workflow{Name: "pto-b", Steps: []Step{
			{ID: "hold", Verb: VerbHold},
			{ID: "ok", Verb: VerbApprove, On: map[string]string{"accepted": "notify", "declined": "denied"}},
			{ID: "notify", Verb: VerbNotify, Next: "end"},
			{ID: "denied", Verb: VerbNote, Subject: "Declined", Next: "end"},
		}},
		Want: nil,
	}, { // Test 18: A branch to an unknown step is invalid.
		Name: "unknown branch target",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{ID: "hold", Verb: VerbHold},
			{ID: "ok", Verb: VerbApprove, On: map[string]string{"accepted": "ghost"}},
		}},
		Want: ErrInvalid,
	}, { // Test 19: on on a non-approve step is invalid.
		Name: "on not on approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold, On: map[string]string{"accepted": "end"}},
		}},
		Want: ErrInvalid,
	}, { // Test 20: An invalid branch outcome is invalid.
		Name: "bad outcome",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{ID: "h", Verb: VerbHold},
			{Verb: VerbApprove, On: map[string]string{"maybe": "end"}},
		}},
		Want: ErrInvalid,
	}, { // Test 21: Duplicate step ids are invalid.
		Name: "duplicate id",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{ID: "a", Verb: VerbHold}, {ID: "a", Verb: VerbNotify},
		}},
		Want: ErrInvalid,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			err := test.Workflow.Validate()
			if !errors.Is(err, test.Want) {
				t.Errorf("%s: Validate = %v, want %v", test.Name, err, test.Want)
			}
		})
	}
}

func TestVerbClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Verb        Verb
		WantCreates bool
		WantWaits   bool
	}{{ // Test 0: Hold creates and does not wait.
		Verb: VerbHold, WantCreates: true, WantWaits: false,
	}, { // Test 1: Away creates.
		Verb: VerbAway, WantCreates: true, WantWaits: false,
	}, { // Test 2: Event creates.
		Verb: VerbEvent, WantCreates: true, WantWaits: false,
	}, { // Test 3: Approve waits and does not create.
		Verb: VerbApprove, WantCreates: false, WantWaits: true,
	}, { // Test 4: Notify neither creates nor waits.
		Verb: VerbNotify, WantCreates: false, WantWaits: false,
	}, { // Test 5: Cancel neither creates nor waits.
		Verb: VerbCancel, WantCreates: false, WantWaits: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.Verb.Creates(); got != test.WantCreates {
				t.Errorf("%s.Creates() = %v, want %v", test.Verb, got, test.WantCreates)
			}
			if got := test.Verb.Waits(); got != test.WantWaits {
				t.Errorf("%s.Waits() = %v, want %v", test.Verb, got, test.WantWaits)
			}
		})
	}
}

func TestNext(t *testing.T) {
	t.Parallel()
	branch := Workflow{Steps: []Step{
		{ID: "hold", Verb: VerbHold},
		{ID: "ok", Verb: VerbApprove, On: map[string]string{"accepted": "notify", "declined": "denied"}},
		{ID: "notify", Verb: VerbNotify, Next: "end"},
		{ID: "denied", Verb: VerbNote, Next: "end"},
	}}
	linear := Workflow{Steps: []Step{{Verb: VerbHold}, {Verb: VerbApprove}, {Verb: VerbNotify}}}
	tests := []struct {
		Name     string
		Workflow Workflow
		I        int
		Outcome  string
		Want     int
	}{{ // Test 0: A plain step falls through to the next.
		Name: "fall through", Workflow: branch, I: 0, Outcome: "", Want: 1,
	}, { // Test 1: Accepted follows the accepted branch.
		Name: "accepted branch", Workflow: branch, I: 1, Outcome: "accepted", Want: 2,
	}, { // Test 2: Declined follows the declined branch.
		Name: "declined branch", Workflow: branch, I: 1, Outcome: "declined", Want: 3,
	}, { // Test 3: next end stops.
		Name: "next end", Workflow: branch, I: 2, Outcome: "", Want: -1,
	}, { // Test 4: A linear accept with no branch falls through.
		Name: "linear accept", Workflow: linear, I: 1, Outcome: "accepted", Want: 2,
	}, { // Test 5: Past the end stops.
		Name: "past end", Workflow: linear, I: 2, Outcome: "", Want: -1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.Workflow.Next(test.I, test.Outcome); got != test.Want {
				t.Errorf("%s: Next(%d, %q) = %d, want %d", test.Name, test.I, test.Outcome, got, test.Want)
			}
		})
	}
}

// TestValidateTimeout covers the approve-step timeout and expired-branch rules.
func TestValidateTimeout(t *testing.T) {
	t.Parallel()
	hold := Step{ID: "hold", Verb: VerbHold}
	tests := []struct {
		Approve Step
		WantErr bool
	}{{ // Test 0: A valid timeout with an expired branch passes.
		Approve: Step{ID: "a", Verb: VerbApprove, Timeout: "72h", On: map[string]string{"accepted": "end", "expired": "end"}},
	}, { // Test 1: A timeout without an expired branch fails.
		Approve: Step{ID: "a", Verb: VerbApprove, Timeout: "72h", On: map[string]string{"accepted": "end"}}, WantErr: true,
	}, { // Test 2: An expired branch without a timeout fails.
		Approve: Step{ID: "a", Verb: VerbApprove, On: map[string]string{"expired": "end"}}, WantErr: true,
	}, { // Test 3: An unparseable timeout fails.
		Approve: Step{ID: "a", Verb: VerbApprove, Timeout: "soon", On: map[string]string{"expired": "end"}}, WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			wf := Workflow{Name: "t", Steps: []Step{hold, test.Approve}}
			if err := wf.Validate(); (err != nil) != test.WantErr {
				t.Errorf("Validate err = %v, wantErr %v", err, test.WantErr)
			}
		})
	}
}

// TestValidateWhen covers the when-guard rules on a non-creating step.
func TestValidateWhen(t *testing.T) {
	t.Parallel()
	hold := Step{ID: "hold", Verb: VerbHold}
	tests := []struct {
		Notify  Step
		WantErr bool
	}{{ // Test 0: A valid day-of-week guard passes.
		Notify: Step{Verb: VerbNotify, When: When{DayOfWeek: "mon-fri"}},
	}, { // Test 1: A valid attendee-count guard passes.
		Notify: Step{Verb: VerbNotify, When: When{MinAttendees: 1, MaxAttendees: 5}},
	}, { // Test 2: An unparseable day set fails.
		Notify: Step{Verb: VerbNotify, When: When{DayOfWeek: "someday"}}, WantErr: true,
	}, { // Test 3: Min greater than max fails.
		Notify: Step{Verb: VerbNotify, When: When{MinAttendees: 5, MaxAttendees: 2}}, WantErr: true,
	}, { // Test 4: A negative bound fails.
		Notify: Step{Verb: VerbNotify, When: When{MaxAttendees: -1}}, WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			wf := Workflow{Name: "t", Steps: []Step{hold, test.Notify}}
			if err := wf.Validate(); (err != nil) != test.WantErr {
				t.Errorf("Validate err = %v, wantErr %v", err, test.WantErr)
			}
		})
	}
}

// TestValidateWhenOnCreator confirms guarding the creating step is rejected, since a
// workflow must create its hold.
func TestValidateWhenOnCreator(t *testing.T) {
	t.Parallel()
	wf := Workflow{Name: "t", Steps: []Step{
		{ID: "hold", Verb: VerbHold, When: When{DayOfWeek: "mon-fri"}},
		{Verb: VerbNotify},
	}}
	if err := wf.Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("Validate err = %v, want ErrInvalid", err)
	}
}

// TestParseDays covers the day-of-week set parser: ranges, lists, wrapping, and the
// error cases.
func TestParseDays(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Spec    string
		Want    map[time.Weekday]bool
		WantErr bool
	}{{ // Test 0: A weekday range expands inclusively.
		Spec: "mon-fri", Want: daySet(time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday),
	}, { // Test 1: A comma list names individual days.
		Spec: "sat,sun", Want: daySet(time.Saturday, time.Sunday),
	}, { // Test 2: A single day.
		Spec: "wed", Want: daySet(time.Wednesday),
	}, { // Test 3: A mixed list and range, case-insensitive with spaces.
		Spec: "Mon, wed-thu", Want: daySet(time.Monday, time.Wednesday, time.Thursday),
	}, { // Test 4: A range wraps past Saturday.
		Spec: "fri-mon", Want: daySet(time.Friday, time.Saturday, time.Sunday, time.Monday),
	}, { // Test 5: An unknown day errors.
		Spec: "mon-funday", WantErr: true,
	}, { // Test 6: An empty spec errors.
		Spec: "", WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got, err := parseDays(test.Spec)
			if (err != nil) != test.WantErr {
				t.Fatalf("parseDays(%q) err = %v, wantErr %v", test.Spec, err, test.WantErr)
			}
			if test.WantErr {
				return
			}
			if !reflect.DeepEqual(got, test.Want) {
				t.Errorf("parseDays(%q) = %v, want %v", test.Spec, got, test.Want)
			}
		})
	}
}

// TestWhenAllows covers the guard evaluation across day-of-week and attendee bounds.
func TestWhenAllows(t *testing.T) {
	t.Parallel()
	wed := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	sat := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		When      When
		At        time.Time
		Attendees int
		Want      bool
	}{{ // Test 0: An empty guard always allows.
		When: When{}, At: sat, Want: true,
	}, { // Test 1: A weekday guard allows on a weekday.
		When: When{DayOfWeek: "mon-fri"}, At: wed, Want: true,
	}, { // Test 2: A weekday guard denies on the weekend.
		When: When{DayOfWeek: "mon-fri"}, At: sat, Want: false,
	}, { // Test 3: Min attendees met.
		When: When{MinAttendees: 3}, At: wed, Attendees: 3, Want: true,
	}, { // Test 4: Min attendees not met.
		When: When{MinAttendees: 3}, At: wed, Attendees: 2, Want: false,
	}, { // Test 5: Max attendees respected.
		When: When{MaxAttendees: 2}, At: wed, Attendees: 2, Want: true,
	}, { // Test 6: Max attendees exceeded.
		When: When{MaxAttendees: 2}, At: wed, Attendees: 3, Want: false,
	}, { // Test 7: All conditions must hold together.
		When: When{DayOfWeek: "mon-fri", MinAttendees: 2, MaxAttendees: 5}, At: wed, Attendees: 3, Want: true,
	}, { // Test 8: One failing condition denies.
		When: When{DayOfWeek: "mon-fri", MinAttendees: 2}, At: sat, Attendees: 3, Want: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.When.Allows(test.At, test.Attendees); got != test.Want {
				t.Errorf("Allows = %v, want %v", got, test.Want)
			}
		})
	}
}

// daySet builds a weekday set for comparing parseDays output.
func daySet(days ...time.Weekday) map[time.Weekday]bool {
	m := make(map[time.Weekday]bool, len(days))
	for _, d := range days {
		m[d] = true
	}
	return m
}
