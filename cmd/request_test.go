package cmd

import (
	"fmt"
	"testing"
)

func TestParseWhen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		In         string
		WantAllDay bool
		WantErr    bool
	}{{ // Test 0: RFC3339 timestamp is not all-day.
		In: "2026-07-20T09:00:00Z", WantAllDay: false, WantErr: false,
	}, { // Test 1: Bare date is all-day.
		In: "2026-07-20", WantAllDay: true, WantErr: false,
	}, { // Test 2: Garbage input errors.
		In: "next tuesday", WantAllDay: false, WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			_, allDay, err := parseWhen(test.In)
			if (err != nil) != test.WantErr {
				t.Fatalf("parseWhen(%q) err = %v, wantErr %v", test.In, err, test.WantErr)
			}
			if err != nil {
				return
			}
			if allDay != test.WantAllDay {
				t.Errorf("parseWhen(%q) allDay = %v, want %v", test.In, allDay, test.WantAllDay)
			}
		})
	}
}
