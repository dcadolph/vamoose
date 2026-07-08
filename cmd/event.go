package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runEvent creates a quick calendar event with no approval or team fanout,
// optionally inviting attendees.
func runEvent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	var (
		start     = fs.String("start", "", "Start date/time, RFC3339 or YYYY-MM-DD (required)")
		end       = fs.String("end", "", "End date/time, RFC3339 or YYYY-MM-DD (required)")
		subject   = fs.String("subject", "", "Event subject (required)")
		body      = fs.String("body", "", "Event description")
		attendees = fs.String("attendees", "", "Comma-separated attendee emails to invite")
		free      = fs.Bool("free", false, "Show the event as free instead of busy")
		provider  = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag    = fs.String("tz", "", "IANA time zone for event times")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *subject == "" || *start == "" || *end == "" {
		fs.Usage()
		return fmt.Errorf("event: --start, --end, and --subject are required")
	}
	startAt, endAt, allDay, err := parseWindow(*start, *end)
	if err != nil {
		return fmt.Errorf("event: %w", err)
	}
	prov, err := newProvider(resolveProvider(*provider), resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}
	showAs := calendar.ShowBusy
	if *free {
		showAs = calendar.ShowFree
	}
	hold := calendar.Hold{
		Subject:   *subject,
		Body:      *body,
		Start:     startAt,
		End:       endAt,
		AllDay:    allDay,
		ShowAs:    showAs,
		Attendees: attendeesFromCSV(*attendees),
	}
	created, err := prov.CreateHold(ctx, hold)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Created %q %s to %s.\nEvent id: %s\n",
		*subject, startAt.Format(time.RFC3339), endAt.Format(time.RFC3339), created.ID)
	return nil
}

// attendeesFromCSV builds required attendees from a comma-separated email list,
// dropping blank entries.
func attendeesFromCSV(csv string) []calendar.Attendee {
	people := peopleFromEmails(strings.Split(csv, ","))
	if len(people) == 0 {
		return nil
	}
	out := make([]calendar.Attendee, 0, len(people))
	for _, p := range people {
		out = append(out, calendar.Attendee{Person: p, Role: calendar.RoleRequired})
	}
	return out
}
