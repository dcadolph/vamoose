package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runRequest creates a vacation hold shown as free and invites the manager to approve it.
func runRequest(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("request", flag.ContinueOnError)
	var (
		start   = fs.String("start", "", "Departure date/time, RFC3339 or YYYY-MM-DD (required)")
		end     = fs.String("end", "", "Return date/time, RFC3339 or YYYY-MM-DD (required)")
		subject = fs.String("subject", "", "Event subject (required)")
		body    = fs.String("body", "", "Event description")
		manager = fs.String("manager", "", "Manager email; resolved from the directory when empty")
		tzFlag  = fs.String("tz", "", "IANA time zone for event times")
		dryRun  = fs.Bool("dry-run", false, "Print what would be sent without calling the calendar")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *subject == "" || *start == "" || *end == "" {
		fs.Usage()
		return fmt.Errorf("request: --start, --end, and --subject are required")
	}

	startAt, startAllDay, err := parseWhen(*start)
	if err != nil {
		return fmt.Errorf("parse start: %w", err)
	}
	endAt, endAllDay, err := parseWhen(*end)
	if err != nil {
		return fmt.Errorf("parse end: %w", err)
	}
	allDay := startAllDay && endAllDay
	if allDay {
		endAt = endAt.AddDate(0, 0, 1) // Graph's all-day end is exclusive.
	}
	if !endAt.After(startAt) {
		return fmt.Errorf("request: end must be after start")
	}

	prov, err := newProvider(resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}

	mgr := calendar.Person{Email: *manager}
	if mgr.Email == "" {
		resolved, merr := prov.Manager(ctx)
		if merr != nil {
			return fmt.Errorf("resolve manager: %w", merr)
		}
		mgr = resolved
	}

	hold := calendar.Hold{
		Subject: *subject,
		Body:    *body,
		Start:   startAt,
		End:     endAt,
		AllDay:  allDay,
		ShowAs:  calendar.ShowFree,
		Attendees: []calendar.Attendee{
			{Person: mgr, Role: calendar.RoleRequired},
		},
	}

	if *dryRun {
		fmt.Fprintf(os.Stdout, "would create hold %q %s -> %s, inviting %s\n",
			hold.Subject, startAt.Format(time.RFC3339), endAt.Format(time.RFC3339), mgr.Email)
		return nil
	}

	created, err := prov.CreateHold(ctx, hold)
	if err != nil {
		return fmt.Errorf("create hold: %w", err)
	}
	if err := saveState(state{LastHoldID: created.ID}); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Hold created and sent to %s for approval.\nHold id: %s\nCheck status with: vamoose check\n",
		mgr.Email, created.ID)
	return nil
}

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
