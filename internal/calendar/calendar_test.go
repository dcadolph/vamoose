package calendar

import (
	"fmt"
	"testing"
)

func TestHoldApprovedBy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Email      string
		Attendees  []Attendee
		WantResult bool
	}{{ // Test 0: Manager accepted.
		Email: "mgr@x.com",
		Attendees: []Attendee{
			{Person: Person{Email: "mgr@x.com"}, Role: RoleRequired, Response: ResponseAccepted},
		},
		WantResult: true,
	}, { // Test 1: Manager not responded.
		Email: "mgr@x.com",
		Attendees: []Attendee{
			{Person: Person{Email: "mgr@x.com"}, Role: RoleRequired, Response: ResponseNotResponded},
		},
		WantResult: false,
	}, { // Test 2: Match is case-insensitive.
		Email: "MGR@X.com",
		Attendees: []Attendee{
			{Person: Person{Email: "mgr@x.com"}, Role: RoleRequired, Response: ResponseAccepted},
		},
		WantResult: true,
	}, { // Test 3: Email not present.
		Email:      "ghost@x.com",
		Attendees:  []Attendee{{Person: Person{Email: "mgr@x.com"}, Response: ResponseAccepted}},
		WantResult: false,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			h := Hold{Attendees: test.Attendees}
			if got := h.ApprovedBy(test.Email); got != test.WantResult {
				t.Errorf("ApprovedBy(%q) = %v, want %v", test.Email, got, test.WantResult)
			}
		})
	}
}
