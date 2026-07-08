package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runAway marks the signed-in user out of office over a window. It creates an
// out-of-office event with no attendees, so there is no approval or fanout.
func runAway(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("away", flag.ContinueOnError)
	var (
		start    = fs.String("start", "", "Away start date/time, RFC3339 or YYYY-MM-DD (required)")
		end      = fs.String("end", "", "Away end date/time, RFC3339 or YYYY-MM-DD (required)")
		subject  = fs.String("subject", "Out of office", "Event subject")
		provider = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag   = fs.String("tz", "", "IANA time zone for event times")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *start == "" || *end == "" {
		fs.Usage()
		return fmt.Errorf("away: --start and --end are required")
	}
	startAt, endAt, allDay, err := parseWindow(*start, *end)
	if err != nil {
		return fmt.Errorf("away: %w", err)
	}
	prov, err := newProvider(resolveProvider(*provider), resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}
	hold := calendar.Hold{
		Subject: *subject,
		Start:   startAt,
		End:     endAt,
		AllDay:  allDay,
		ShowAs:  calendar.ShowOOF,
	}
	created, err := prov.CreateHold(ctx, hold)
	if err != nil {
		return fmt.Errorf("create away event: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Marked out of office %s to %s.\nEvent id: %s\n",
		startAt.Format(time.RFC3339), endAt.Format(time.RFC3339), created.ID)
	return nil
}
