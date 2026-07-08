package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runCheck reports the manager's response to a hold and optionally promotes it.
func runCheck(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	var (
		id       = fs.String("id", "", "Hold id; defaults to the most recent hold")
		provider = fs.String("provider", "", "Calendar provider for an explicit --id; overrides VAMOOSE_PROVIDER")
		tzFlag   = fs.String("tz", "", "IANA time zone")
		auto     = fs.Bool("promote", false, "Promote to the team automatically when approved")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ref, err := resolveHold(*id, *provider)
	if err != nil {
		return err
	}
	prov, err := newProvider(ref.Provider, resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}
	hold, err := prov.GetHold(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("get hold: %w", err)
	}

	mgr := managerAttendee(hold)
	switch mgr.Response {
	case calendar.ResponseAccepted:
		fmt.Fprintf(os.Stdout, "Approved by %s.\n", mgr.Person.Email)
		if *auto {
			return promoteHold(ctx, prov, hold)
		}
		fmt.Fprintln(os.Stdout, "Fan out to the team with: vamoose promote")
	case calendar.ResponseDeclined:
		fmt.Fprintf(os.Stdout, "Declined by %s.\n", mgr.Person.Email)
	default:
		who := "your manager"
		if mgr.Person.Email != "" {
			who = mgr.Person.Email
		}
		fmt.Fprintf(os.Stdout, "Waiting on %s (status: %s).\n", who, mgr.Response)
	}
	return nil
}

// managerAttendee returns the attendee treated as the approver: the first
// required attendee, or the first attendee when none are required.
func managerAttendee(h calendar.Hold) calendar.Attendee {
	for _, a := range h.Attendees {
		if a.Role == calendar.RoleRequired {
			return a
		}
	}
	if len(h.Attendees) > 0 {
		return h.Attendees[0]
	}
	return calendar.Attendee{}
}
