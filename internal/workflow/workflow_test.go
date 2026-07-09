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
	}, { // Test 15: A non-notify step after approval is invalid.
		Name: "cancel after approve",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbHold}, {Verb: VerbApprove}, {Verb: VerbCancel},
		}},
		Want: ErrInvalid,
	}, { // Test 16: Away then notify with no approval is valid.
		Name: "away then notify",
		Workflow: Workflow{Name: "x", Steps: []Step{
			{Verb: VerbAway}, {Verb: VerbNotify},
		}},
		Want: nil,
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
