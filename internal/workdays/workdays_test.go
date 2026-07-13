package workdays

import (
	"fmt"
	"testing"
	"time"
)

// day builds a UTC date for the test tables.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestCount covers working-day counting across weekends, holidays, custom weekends, and
// the exclusive end convention.
func TestCount(t *testing.T) {
	t.Parallel()
	// 2026-08-03 is a Monday.
	mon := day(2026, 8, 3)
	tests := []struct {
		Name  string
		Set   *Set
		Start time.Time
		End   time.Time
		Want  int
	}{{ // Test 0: Monday through Friday (end exclusive Saturday) is five working days.
		Name: "full week", Set: New(), Start: mon, End: mon.AddDate(0, 0, 5), Want: 5,
	}, { // Test 1: Friday through Monday (exclusive) is one working day, skipping the weekend.
		Name: "over a weekend", Set: New(), Start: mon.AddDate(0, 0, 4), End: mon.AddDate(0, 0, 7), Want: 1,
	}, { // Test 2: A single Saturday is zero working days.
		Name: "just saturday", Set: New(), Start: mon.AddDate(0, 0, 5), End: mon.AddDate(0, 0, 6), Want: 0,
	}, { // Test 3: A holiday inside the week is not counted.
		Name: "with a holiday", Set: New().WithHolidays(mon.AddDate(0, 0, 2)), Start: mon, End: mon.AddDate(0, 0, 5), Want: 4,
	}, { // Test 4: A Friday-Saturday weekend counts Sunday as a working day.
		Name: "custom weekend", Set: New().WithWeekend(time.Friday, time.Saturday), Start: mon, End: mon.AddDate(0, 0, 7), Want: 5,
	}, { // Test 5: No weekend at all counts every day.
		Name: "no weekend", Set: New().WithWeekend(), Start: mon, End: mon.AddDate(0, 0, 7), Want: 7,
	}, { // Test 6: An empty window is zero.
		Name: "empty", Set: New(), Start: mon, End: mon, Want: 0,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := test.Set.Count(test.Start, test.End); got != test.Want {
				t.Errorf("%s: Count = %d, want %d", test.Name, got, test.Want)
			}
		})
	}
}

// TestIsWorkday covers the three day kinds.
func TestIsWorkday(t *testing.T) {
	t.Parallel()
	s := New().WithHolidays(day(2026, 12, 25))
	tests := []struct {
		Name string
		Day  time.Time
		Want bool
	}{
		{"weekday", day(2026, 8, 3), true},    // Monday
		{"saturday", day(2026, 8, 8), false},  // Saturday
		{"sunday", day(2026, 8, 9), false},    // Sunday
		{"holiday", day(2026, 12, 25), false}, // Christmas, a Friday
	}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			if got := s.IsWorkday(test.Day); got != test.Want {
				t.Errorf("%s: IsWorkday = %v, want %v", test.Name, got, test.Want)
			}
		})
	}
}

// TestFromEnv covers parsing weekend and holiday configuration and the error cases.
func TestFromEnv(t *testing.T) {
	t.Parallel()
	mon := day(2026, 8, 3)
	tests := []struct {
		Name    string
		Env     map[string]string
		WantErr bool
		// Probe checks a resulting behavior when there is no error.
		Probe func(*Set) bool
	}{{ // Test 0: Defaults give a Saturday and Sunday weekend.
		Name: "defaults", Probe: func(s *Set) bool { return s.Count(mon, mon.AddDate(0, 0, 7)) == 5 },
	}, { // Test 1: A custom weekend is honored.
		Name: "custom weekend", Env: map[string]string{"VAMOOSE_WEEKEND": "fri,sat"},
		Probe: func(s *Set) bool { return !s.IsWorkday(day(2026, 8, 7)) && s.IsWorkday(day(2026, 8, 9)) },
	}, { // Test 2: Holidays are parsed and excluded.
		Name: "holidays", Env: map[string]string{"VAMOOSE_HOLIDAYS": "2026-12-25, 2027-01-01"},
		Probe: func(s *Set) bool { return !s.IsWorkday(day(2026, 12, 25)) },
	}, { // Test 3: A bad weekday name errors.
		Name: "bad weekend", Env: map[string]string{"VAMOOSE_WEEKEND": "funday"}, WantErr: true,
	}, { // Test 4: A bad holiday date errors.
		Name: "bad holiday", Env: map[string]string{"VAMOOSE_HOLIDAYS": "2026-13-40"}, WantErr: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			s, err := FromEnv(func(k string) string { return test.Env[k] })
			if (err != nil) != test.WantErr {
				t.Fatalf("%s: err = %v, wantErr %v", test.Name, err, test.WantErr)
			}
			if test.WantErr {
				return
			}
			if test.Probe != nil && !test.Probe(s) {
				t.Errorf("%s: probe failed", test.Name)
			}
		})
	}
}
