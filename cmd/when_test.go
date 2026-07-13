package cmd

import (
	"fmt"
	"testing"
	"time"
)

// TestHalfDayWindow covers the morning and afternoon windows, the aliases, and an
// invalid portion.
func TestHalfDayWindow(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	day := time.Date(2026, 8, 3, 0, 0, 0, 0, loc)
	tests := []struct {
		Portion   string
		WantStart string
		WantEnd   string
		WantErr   bool
	}{
		{Portion: "am", WantStart: "09:00", WantEnd: "13:00"},
		{Portion: "morning", WantStart: "09:00", WantEnd: "13:00"},
		{Portion: "pm", WantStart: "13:00", WantEnd: "17:00"},
		{Portion: "afternoon", WantStart: "13:00", WantEnd: "17:00"},
		{Portion: "evening", WantErr: true},
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			s, e, err := halfDayWindow(day, loc, test.Portion)
			if (err != nil) != test.WantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.WantErr)
			}
			if test.WantErr {
				return
			}
			if got := s.Format("15:04"); got != test.WantStart {
				t.Errorf("start = %s, want %s", got, test.WantStart)
			}
			if got := e.Format("15:04"); got != test.WantEnd {
				t.Errorf("end = %s, want %s", got, test.WantEnd)
			}
		})
	}
}

// TestHalfDayWindowKeepsDate confirms the calendar day does not shift when the source
// value is midnight in one zone and the window is built in another behind it.
func TestHalfDayWindowKeepsDate(t *testing.T) {
	t.Parallel()
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	// Midnight UTC on Aug 3; viewed from Chicago this is the evening of Aug 2.
	midnightUTC := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	s, _, err := halfDayWindow(midnightUTC, chicago, "am")
	if err != nil {
		t.Fatalf("halfDayWindow: %v", err)
	}
	if got := s.Format("2006-01-02"); got != "2026-08-03" {
		t.Errorf("half-day date = %s, want 2026-08-03 (must not shift to Aug 2)", got)
	}
	if got := s.Format("15:04"); got != "09:00" {
		t.Errorf("half-day start = %s, want 09:00 in the provider zone", got)
	}
}

// TestApplyHalfDay confirms a single day is narrowed and a multi-day range is rejected.
func TestApplyHalfDay(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	if _, _, err := applyHalfDay(start, start.AddDate(0, 0, 3), time.UTC, "am"); err == nil {
		t.Error("want an error for a multi-day range")
	}
	s, e, err := applyHalfDay(start, start.AddDate(0, 0, 1), time.UTC, "pm")
	if err != nil {
		t.Fatalf("single day should be accepted: %v", err)
	}
	if s.Format("15:04") != "13:00" || e.Format("15:04") != "17:00" {
		t.Errorf("pm window = %s-%s, want 13:00-17:00", s.Format("15:04"), e.Format("15:04"))
	}
}

// TestApplyHalfDayDSTDay confirms a single calendar day that is 25 hours long, on the
// autumn daylight-saving change, is still accepted rather than read as a range.
func TestApplyHalfDayDSTDay(t *testing.T) {
	t.Parallel()
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("no tzdata: %v", err)
	}
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, la) // US clocks fall back this day
	end := start.AddDate(0, 0, 1)                    // next midnight, 25 hours later
	if end.Sub(start) <= 24*time.Hour {
		t.Skip("tzdata does not model the DST change here")
	}
	if _, _, err := applyHalfDay(start, end, la, "am"); err != nil {
		t.Errorf("a single 25-hour DST day should be accepted, got %v", err)
	}
}

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
