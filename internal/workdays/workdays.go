// Package workdays counts the working days in a date window, excluding weekends and a
// configured set of holidays. Time off is spent in working days, not calendar days, so a
// Friday-to-Monday block is one working day off, not four, and a public holiday inside a
// range is not deducted. The set is pure and configuration-driven so it is easy to test.
package workdays

import (
	"fmt"
	"strings"
	"time"
)

// dateLayout is how holiday dates are written and compared, as YYYY-MM-DD.
const dateLayout = "2006-01-02"

// shortDays maps the three-letter weekday names used in configuration to their weekday.
var shortDays = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

// Set records which days do not count as working days: the weekend weekdays and a set of
// specific holiday dates.
type Set struct {
	// weekend holds the weekdays that are not worked.
	weekend map[time.Weekday]bool
	// holidays holds specific non-working dates, keyed by YYYY-MM-DD.
	holidays map[string]bool
}

// New returns a Set with the default weekend of Saturday and Sunday and no holidays.
func New() *Set {
	return &Set{
		weekend:  map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
		holidays: map[string]bool{},
	}
}

// WithWeekend replaces the weekend with the given weekdays. Passing none clears the
// weekend, so every weekday is worked.
func (s *Set) WithWeekend(days ...time.Weekday) *Set {
	s.weekend = make(map[time.Weekday]bool, len(days))
	for _, d := range days {
		s.weekend[d] = true
	}
	return s
}

// WithHolidays adds the given dates as holidays. Each is compared by calendar date.
func (s *Set) WithHolidays(dates ...time.Time) *Set {
	for _, d := range dates {
		s.holidays[d.Format(dateLayout)] = true
	}
	return s
}

// IsWorkday reports whether t falls on a working day: not a weekend day and not a holiday.
func (s *Set) IsWorkday(t time.Time) bool {
	if s.weekend[t.Weekday()] {
		return false
	}
	return !s.holidays[t.Format(dateLayout)]
}

// Count returns the number of working days in the window [start, end), with the end
// exclusive, matching how vamoose represents an all-day span. It counts by calendar day
// in start's location, so a partial start or end day still counts as its whole day.
func (s *Set) Count(start, end time.Time) int {
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	n := 0
	for day.Before(end) {
		if s.IsWorkday(day) {
			n++
		}
		day = day.AddDate(0, 0, 1)
	}
	return n
}

// FromEnv builds a Set from configuration. VAMOOSE_WEEKEND is a comma-separated list of
// three-letter weekday names that overrides the default Saturday and Sunday weekend.
// VAMOOSE_HOLIDAYS is a comma-separated list of YYYY-MM-DD dates. An unparseable value is
// an error so a typo does not silently miscount leave.
func FromEnv(getenv func(string) string) (*Set, error) {
	s := New()
	if w := strings.TrimSpace(getenv("VAMOOSE_WEEKEND")); w != "" {
		days, err := parseWeekend(w)
		if err != nil {
			return nil, err
		}
		s.WithWeekend(days...)
	}
	if h := strings.TrimSpace(getenv("VAMOOSE_HOLIDAYS")); h != "" {
		for _, part := range strings.Split(h, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			d, err := time.Parse(dateLayout, part)
			if err != nil {
				return nil, fmt.Errorf("holiday %q: want YYYY-MM-DD", part)
			}
			s.holidays[d.Format(dateLayout)] = true
		}
	}
	return s, nil
}

// parseWeekend turns a comma-separated list of three-letter weekday names into weekdays.
func parseWeekend(list string) ([]time.Weekday, error) {
	var out []time.Weekday
	for _, part := range strings.Split(list, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		wd, ok := shortDays[part]
		if !ok {
			return nil, fmt.Errorf("weekend day %q: want three-letter names like sat,sun", part)
		}
		out = append(out, wd)
	}
	return out, nil
}
