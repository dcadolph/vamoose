package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
)

// runCancel deletes a hold, notifies its attendees, and stops watching it.
func runCancel(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	var (
		id       = fs.String("id", "", "Hold id; defaults to the most recent hold")
		provider = fs.String("provider", "", "Calendar provider for an explicit --id; overrides VAMOOSE_PROVIDER")
		tzFlag   = fs.String("tz", "", "IANA time zone")
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
	if err := prov.DeleteHold(ctx, ref.ID); err != nil {
		return fmt.Errorf("cancel hold: %w", err)
	}
	forgetHold(ref)
	fmt.Fprintf(os.Stdout, "Canceled hold %s and notified attendees.\n", ref.ID)
	return nil
}

// forgetHold drops a canceled hold from the watch list and clears it from state
// when it was the last created hold. Best-effort: the cancellation already ran.
func forgetHold(ref holdRef) {
	_ = removeWatch(ref.Provider, ref.ID)
	forgetCoverage(ref.ID)
	if s, err := loadState(); err == nil && s.LastHold == ref {
		_ = saveState(state{})
	}
}
