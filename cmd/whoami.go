package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dcadolph/vamoose/internal/calendar"
)

// runWhoami prints the signed-in user, their manager, and the resolved team.
// It is the fastest way to confirm auth and directory access work in a tenant.
func runWhoami(ctx context.Context, _ []string) error {
	prov, err := newProvider(resolveTimeZone(""))
	if err != nil {
		return err
	}

	me, err := prov.Me(ctx)
	if err != nil {
		return fmt.Errorf("resolve me: %w", err)
	}
	fmt.Fprintf(os.Stdout, "me:      %s\n", personLabel(me))

	mgr, err := prov.Manager(ctx)
	switch {
	case errors.Is(err, calendar.ErrNoManager):
		fmt.Fprintln(os.Stdout, "manager: none set in directory")
	case err != nil:
		return fmt.Errorf("resolve manager: %w", err)
	default:
		fmt.Fprintf(os.Stdout, "manager: %s\n", personLabel(mgr))
	}

	team, source, err := resolveTeam(ctx, prov)
	if err != nil {
		return fmt.Errorf("resolve team: %w", err)
	}
	fmt.Fprintf(os.Stdout, "team:    %d (%s)\n", len(team), source)
	for _, p := range team {
		fmt.Fprintf(os.Stdout, "  %s\n", personLabel(p))
	}
	return nil
}
