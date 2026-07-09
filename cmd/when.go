package cmd

import (
	"fmt"
	"strings"
	"time"
)

// parseWhen parses an RFC3339 timestamp or a YYYY-MM-DD date. A bare date is
// treated as all-day and returns true.
func parseWhen(s string) (time.Time, bool, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, false, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("unrecognized time %q", s)
}

// parseWindow parses start and end into a time window. Two bare dates form an
// all-day span whose end is made exclusive, matching how calendars treat them.
func parseWindow(start, end string) (startAt, endAt time.Time, allDay bool, err error) {
	startAt, startAllDay, err := parseWhen(start)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse start: %w", err)
	}
	endAt, endAllDay, err := parseWhen(end)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("parse end: %w", err)
	}
	allDay = startAllDay && endAllDay
	if allDay {
		endAt = endAt.AddDate(0, 0, 1)
	}
	if !endAt.After(startAt) {
		return time.Time{}, time.Time{}, false, fmt.Errorf("end must be after start")
	}
	return startAt, endAt, allDay, nil
}

// resolveWindow turns explicit start and end flags or a date phrase into a time
// window. Explicit dates win over the phrase; a phrase yields an all-day window.
func resolveWindow(start, end, phrase string) (startAt, endAt time.Time, allDay bool, err error) {
	switch {
	case start != "" && end != "":
		return parseWindow(start, end)
	case phrase != "":
		startAt, endAt, err = resolveRelative(time.Now(), phrase)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		return startAt, endAt, true, nil
	default:
		return time.Time{}, time.Time{}, false, fmt.Errorf("give a date phrase (e.g. \"next week\") or --start and --end")
	}
}

// weekdays maps lowercase weekday names to their time.Weekday.
var weekdays = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

// startOfDay returns midnight at the start of t's day in t's location.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// nextWeekday returns midnight on the next occurrence of target after now, never
// today.
func nextWeekday(now time.Time, target time.Weekday) time.Time {
	d := (int(target) - int(now.Weekday()) + 7) % 7
	if d == 0 {
		d = 7
	}
	return startOfDay(now).AddDate(0, 0, d)
}

// resolveRelative turns a small set of date phrases into an all-day window with
// an exclusive end. Supported: today, tomorrow, next week (the coming Monday
// through Friday), and weekday names optionally prefixed with "next".
func resolveRelative(now time.Time, phrase string) (start, end time.Time, err error) {
	p := strings.ToLower(strings.TrimSpace(phrase))
	today := startOfDay(now)
	switch p {
	case "today":
		return today, today.AddDate(0, 0, 1), nil
	case "tomorrow":
		start = today.AddDate(0, 0, 1)
		return start, start.AddDate(0, 0, 1), nil
	case "next week":
		start = nextWeekday(now, time.Monday)
		return start, start.AddDate(0, 0, 5), nil
	}
	if wd, ok := weekdays[strings.TrimPrefix(p, "next ")]; ok {
		start = nextWeekday(now, wd)
		return start, start.AddDate(0, 0, 1), nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("unrecognized date phrase %q", phrase)
}
