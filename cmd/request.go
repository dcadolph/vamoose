package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
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

// createHold runs the built-in pto workflow: it creates the hold shown free,
// invites the manager, and gates on approval, watching for the daemon when asked.
// request and off are thin fronts over this one workflow.
func createHold(ctx context.Context, req holdRequest) error {
	wf, err := workflowLoader().Load("pto")
	if err != nil {
		return fmt.Errorf("load pto workflow: %w", err)
	}
	return executeWorkflow(ctx, wf, runOptions{
		Provider: req.Provider,
		TZ:       req.TZ,
		Subject:  req.Subject,
		Body:     req.Body,
		Manager:  req.Manager,
		Start:    req.Start,
		End:      req.End,
		AllDay:   req.AllDay,
		DryRun:   req.DryRun,
		Watch:    req.Watch,
	})
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
