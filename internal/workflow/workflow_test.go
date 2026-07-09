package workflow

import (
	"errors"
	"fmt"
	"testing"

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
