package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runPromote adds the team as optional attendees to an approved hold.
func runPromote(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	var (
		id     = fs.String("id", "", "Hold id; defaults to the most recent hold")
		tzFlag = fs.String("tz", "", "IANA time zone")
		force  = fs.Bool("force", false, "Promote even if the manager has not approved")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	holdID, err := resolveHoldID(*id)
	if err != nil {
		return err
	}
	prov, err := newProvider(resolveTimeZone(*tzFlag))
	if err != nil {
		return err
	}
	hold, err := prov.GetHold(ctx, holdID)
	if err != nil {
		return fmt.Errorf("get hold: %w", err)
	}
	mgr := managerAttendee(hold)
	if !*force && mgr.Response != calendar.ResponseAccepted {
		return fmt.Errorf("not approved yet (manager status: %s); use --force to override", mgr.Response)
	}
	return promoteHold(ctx, prov, hold)
}

// promoteHold adds the signed-in user's team as optional attendees and resends.
func promoteHold(ctx context.Context, prov calendar.Provider, hold calendar.Hold) error {
	team, source, err := resolveTeam(ctx, prov)
	if err != nil {
		return fmt.Errorf("resolve team: %w", err)
	}
	seen := make(map[string]bool, len(hold.Attendees))
	for _, a := range hold.Attendees {
		seen[strings.ToLower(a.Person.Email)] = true
	}
	added := 0
	for _, person := range team {
		key := strings.ToLower(person.Email)
		if seen[key] {
			continue
		}
		hold.Attendees = append(hold.Attendees, calendar.Attendee{Person: person, Role: calendar.RoleOptional})
		seen[key] = true
		added++
	}
	if added == 0 {
		fmt.Fprintln(os.Stdout, "No new team members to add; the hold already covers the team.")
		return nil
	}
	if hold.ShowAs == "" {
		hold.ShowAs = calendar.ShowFree
	}
	if _, err := prov.UpdateHold(ctx, hold); err != nil {
		return fmt.Errorf("update hold: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Added %d team member(s) as optional from %s. Everyone notified.\n", added, source)
	return nil
}
