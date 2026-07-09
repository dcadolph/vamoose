package cmd

import (
	"context"
	"errors"
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
		start    = fs.String("start", "", "Departure date/time, RFC3339 or YYYY-MM-DD (required)")
		end      = fs.String("end", "", "Return date/time, RFC3339 or YYYY-MM-DD (required)")
		subject  = fs.String("subject", "", "Event subject (required)")
		body     = fs.String("body", "", "Event description")
		manager  = fs.String("manager", "", "Manager email; resolved from the directory when empty")
		provider = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag   = fs.String("tz", "", "IANA time zone for event times")
		dryRun   = fs.Bool("dry-run", false, "Print what would be sent without calling the calendar")
		watch    = fs.Bool("watch", false, "Add the hold to the daemon watch list for auto-promote on approval")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *subject == "" || *start == "" || *end == "" {
		fs.Usage()
		return fmt.Errorf("request: --start, --end, and --subject are required")
	}
	startAt, endAt, allDay, err := parseWindow(*start, *end)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	return createHold(ctx, holdRequest{
		Provider: *provider,
		TZ:       *tzFlag,
		Subject:  *subject,
		Body:     *body,
		Manager:  *manager,
		Start:    startAt,
		End:      endAt,
		AllDay:   allDay,
		DryRun:   *dryRun,
		Watch:    *watch,
	})
}

// holdRequest describes a vacation hold to create and how to route it.
type holdRequest struct {
	// Provider is the selected calendar provider, empty for the default.
	Provider string
	// TZ is the IANA time zone flag value, empty for the configured default.
	TZ string
	// Subject is the event title.
	Subject string
	// Body is the event description.
	Body string
	// Manager is the approver email; resolved from the directory when empty.
	Manager string
	// Start is the departure time.
	Start time.Time
	// End is the return time, exclusive for all-day holds.
	End time.Time
	// AllDay marks a full-day hold.
	AllDay bool
	// DryRun prints the intended action without calling the calendar.
	DryRun bool
	// Watch adds the created hold to the daemon watch list.
	Watch bool
}

// createHold resolves the manager, creates the hold shown as free, caches it,
// and optionally enqueues it for the daemon to auto-promote.
func createHold(ctx context.Context, req holdRequest) error {
	providerName := resolveProvider(req.Provider)
	prov, err := newProvider(providerName, resolveTimeZone(req.TZ))
	if err != nil {
		return err
	}

	mgr, err := resolveManager(ctx, prov, req.Manager)
	if err != nil {
		return err
	}

	hold := calendar.Hold{
		Subject: req.Subject,
		Body:    req.Body,
		Start:   req.Start,
		End:     req.End,
		AllDay:  req.AllDay,
		ShowAs:  calendar.ShowFree,
		Attendees: []calendar.Attendee{
			{Person: mgr, Role: calendar.RoleRequired},
		},
	}

	if req.DryRun {
		fmt.Fprintf(os.Stdout, "would create hold %q %s -> %s, inviting %s\n",
			hold.Subject, req.Start.Format(time.RFC3339), req.End.Format(time.RFC3339), mgr.Email)
		return nil
	}

	created, err := prov.CreateHold(ctx, hold)
	if err != nil {
		return fmt.Errorf("create hold: %w", err)
	}
	if err := saveState(state{LastHold: holdRef{Provider: providerName, ID: created.ID}}); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Hold created and sent to %s for approval.\nHold id: %s\nCheck status with: vamoose check\n",
		mgr.Email, created.ID)
	if req.Watch {
		pto, werr := workflowLoader().Load("pto")
		if werr != nil {
			return fmt.Errorf("load pto workflow: %w", werr)
		}
		if err := addWatch(watchItem{
			Provider: providerName, HoldID: created.ID,
			Workflow: pto.Name, Step: firstApproveStep(pto), Subject: created.Subject,
		}); err != nil {
			return fmt.Errorf("add watch: %w", err)
		}
		fmt.Fprintln(os.Stdout, "Watching for approval. Run 'vamoose daemon' to auto-promote when approved.")
	}
	return nil
}

// resolveManager returns the approver: the explicit email when given, otherwise the
// manager from the provider directory.
func resolveManager(ctx context.Context, prov calendar.Provider, email string) (calendar.Person, error) {
	if email != "" {
		return calendar.Person{Email: email}, nil
	}
	mgr, err := prov.Manager(ctx)
	if errors.Is(err, calendar.ErrNoManager) {
		return calendar.Person{}, fmt.Errorf("no manager in the directory; pass --manager")
	}
	if err != nil {
		return calendar.Person{}, fmt.Errorf("resolve manager: %w", err)
	}
	return mgr, nil
}
