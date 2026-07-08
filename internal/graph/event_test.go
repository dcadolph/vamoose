package graph

import (
	"fmt"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

func TestResponseFromGraph(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantResult calendar.Response
	}{{ // Test 0: Accepted maps to accepted.
		In: "accepted", WantResult: calendar.ResponseAccepted,
	}, { // Test 1: Organizer counts as accepted.
		In: "organizer", WantResult: calendar.ResponseAccepted,
	}, { // Test 2: Declined maps to declined.
		In: "declined", WantResult: calendar.ResponseDeclined,
	}, { // Test 3: Tentative maps to tentative.
		In: "tentativelyAccepted", WantResult: calendar.ResponseTentative,
	}, { // Test 4: Not responded maps through.
		In: "notResponded", WantResult: calendar.ResponseNotResponded,
	}, { // Test 5: Empty maps to none.
		In: "", WantResult: calendar.ResponseNone,
	}, { // Test 6: Unknown maps to none.
		In: "mystery", WantResult: calendar.ResponseNone,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := responseFromGraph(test.In); got != test.WantResult {
				t.Errorf("responseFromGraph(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}

func TestShowAsRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In calendar.ShowAs
	}{{ // Test 0: Free.
		In: calendar.ShowFree,
	}, { // Test 1: Tentative.
		In: calendar.ShowTentative,
	}, { // Test 2: Busy.
		In: calendar.ShowBusy,
	}, { // Test 3: Out of office.
		In: calendar.ShowOOF,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := showAsFromGraph(showAsToGraph(test.In)); got != test.In {
				t.Errorf("round trip = %q, want %q", got, test.In)
			}
		})
	}
}

func TestRoleToGraph(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         calendar.Role
		WantResult string
	}{{ // Test 0: Optional maps to optional.
		In: calendar.RoleOptional, WantResult: "optional",
	}, { // Test 1: Required maps to required.
		In: calendar.RoleRequired, WantResult: "required",
	}, { // Test 2: Unknown defaults to required.
		In: calendar.Role("weird"), WantResult: "required",
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := roleToGraph(test.In); got != test.WantResult {
				t.Errorf("roleToGraph(%q) = %q, want %q", test.In, got, test.WantResult)
			}
		})
	}
}
