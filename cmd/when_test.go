package cmd

import (
	"fmt"
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Start      string
		End        string
		WantAllDay bool
		WantErr    bool
	}{{ // Test 0: Two bare dates are an all-day span.
		Start: "2026-07-20", End: "2026-07-24", WantAllDay: true, WantErr: false,
	}, { // Test 1: Timed RFC3339 window is not all-day.
		Start: "2026-07-20T09:00:00Z", End: "2026-07-20T17:00:00Z", WantAllDay: false, WantErr: false,
	}, { // Test 2: End not after start errors.
		Start: "2026-07-24", End: "2026-07-20", WantErr: true,
	}, { // Test 3: Unparseable start errors.
		Start: "next week", End: "2026-07-20", WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			startAt, endAt, allDay, err := parseWindow(test.Start, test.End)
			if (err != nil) != test.WantErr {
				t.Fatalf("parseWindow err = %v, wantErr %v", err, test.WantErr)
			}
			if err != nil {
				return
			}
			if allDay != test.WantAllDay {
				t.Errorf("allDay = %v, want %v", allDay, test.WantAllDay)
			}
			if !endAt.After(startAt) {
				t.Errorf("end %v not after start %v", endAt, startAt)
			}
		})
	}
}

func TestResolveRelative(t *testing.T) {
	t.Parallel()
	// Fixed reference date. Assertions use relative properties, not a hardcoded
	// weekday, so they hold regardless of what day the reference falls on.
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	t.Run("today", func(t *testing.T) {
		t.Parallel()
		s, e, err := resolveRelative(now, "today")
		if err != nil {
			t.Fatal(err)
		}
		if !s.Equal(midnight) || !e.Equal(midnight.AddDate(0, 0, 1)) {
			t.Errorf("today = %s..%s", s, e)
		}
	})
	t.Run("tomorrow", func(t *testing.T) {
		t.Parallel()
		s, e, err := resolveRelative(now, "tomorrow")
		if err != nil {
			t.Fatal(err)
		}
		if !s.Equal(midnight.AddDate(0, 0, 1)) || !e.Equal(midnight.AddDate(0, 0, 2)) {
			t.Errorf("tomorrow = %s..%s", s, e)
		}
	})
	t.Run("next week is a Monday-Friday span", func(t *testing.T) {
		t.Parallel()
		s, e, err := resolveRelative(now, "next week")
		if err != nil {
			t.Fatal(err)
		}
		if s.Weekday() != time.Monday {
			t.Errorf("start weekday = %s, want Monday", s.Weekday())
		}
		if !e.Equal(s.AddDate(0, 0, 5)) {
			t.Errorf("end = %s, want start+5", e)
		}
		if !s.After(midnight) {
			t.Errorf("start %s not after the reference day", s)
		}
	})
	t.Run("weekday name is a single day", func(t *testing.T) {
		t.Parallel()
		s, e, err := resolveRelative(now, "friday")
		if err != nil {
			t.Fatal(err)
		}
		if s.Weekday() != time.Friday {
			t.Errorf("weekday = %s, want Friday", s.Weekday())
		}
		if !e.Equal(s.AddDate(0, 0, 1)) {
			t.Errorf("end = %s, want a single day", e)
		}
	})
	t.Run("next prefix", func(t *testing.T) {
		t.Parallel()
		s, _, err := resolveRelative(now, "next monday")
		if err != nil {
			t.Fatal(err)
		}
		if s.Weekday() != time.Monday {
			t.Errorf("weekday = %s, want Monday", s.Weekday())
		}
	})
	t.Run("unknown phrase errors", func(t *testing.T) {
		t.Parallel()
		if _, _, err := resolveRelative(now, "someday"); err == nil {
			t.Error("want error for an unknown phrase")
		}
	})
}
