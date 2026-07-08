package cmd

import (
	"fmt"
	"testing"

	"github.com/dcadolph/vamoose/internal/calendar"
)

func TestAttendeesFromCSV(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In        string
		WantCount int
	}{{ // Test 0: Empty input yields no attendees.
		In: "", WantCount: 0,
	}, { // Test 1: Emails become required attendees.
		In: "a@x.com, b@x.com", WantCount: 2,
	}, { // Test 2: Blank entries are dropped.
		In: "a@x.com,,  ", WantCount: 1,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			got := attendeesFromCSV(test.In)
			if len(got) != test.WantCount {
				t.Fatalf("count = %d, want %d", len(got), test.WantCount)
			}
			for _, a := range got {
				if a.Role != calendar.RoleRequired {
					t.Errorf("role = %q, want required", a.Role)
				}
			}
		})
	}
}
