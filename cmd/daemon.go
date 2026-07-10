package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/workflow"
)

const (
	// defaultInterval is the default polling cadence.
	defaultInterval = time.Minute
	// minInterval is the shortest allowed polling cadence, to avoid hammering.
	minInterval = 10 * time.Second
)

// pollResult is the outcome of polling a single watched hold.
type pollResult int

const (
	// pollPending means the manager has not yet responded.
	pollPending pollResult = iota
	// pollApproved means every approver accepted and the workflow ran to completion.
	pollApproved
	// pollAdvanced means an approver accepted and the chain moved to the next approver,
	// so the hold stays watched at the new gate.
	pollAdvanced
	// pollDeclined means the manager declined.
	pollDeclined
	// pollExpired means the approve step's timeout elapsed and the expired branch ran.
	pollExpired
	// pollFailed means the poll or promotion errored.
	pollFailed
)

// runDaemon polls watched holds and auto-promotes them when the manager approves.
// It runs until interrupted, or does a single pass with --once.
func runDaemon(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	var (
		interval = fs.Duration("interval", defaultInterval, "Polling interval")
		once     = fs.Bool("once", false, "Poll once and exit")
		prune    = fs.Bool("prune", false, "Drop watched holds whose provider cannot be built")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	every := *interval
	if every < minInterval {
		every = minInterval
	}

	logger := log.New(os.Stderr, "vamoose daemon: ", log.LstdFlags)
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !*once {
		logger.Printf("watching for approvals, polling every %s", every)
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	warned := make(map[string]bool)
	for {
		pollAll(ctx, logger, *prune, warned)
		if *once {
			return nil
		}
		select {
		case <-ctx.Done():
			logger.Println("stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// pollAll advances every watched hold once, rewriting the watch list to drop the
// holds that reached a terminal state. A hold whose provider cannot be built is
// dropped when prune is set, otherwise kept and warned about once per item (tracked
// in warned), so an unconfigured provider does not spam the log every poll.
func pollAll(ctx context.Context, logger *log.Logger, prune bool, warned map[string]bool) {
	watches, err := loadWatches()
	if err != nil {
		logger.Printf("load watches: %v", err)
		return
	}
	if len(watches) == 0 {
		logger.Println("no holds to watch")
		return
	}
	remaining := make([]watchItem, 0, len(watches))
	for _, w := range watches {
		prov, perr := newProvider(w.Provider, resolveTimeZone(""))
		if perr != nil {
			if prune {
				logger.Printf("%s: dropping, provider not configured: %v", label(w), perr)
				continue
			}
			key := w.Provider + ":" + w.HoldID
			if !warned[key] {
				logger.Printf("%s: provider not configured, keeping in the watch list (use --prune to drop): %v", label(w), perr)
				warned[key] = true
			}
			remaining = append(remaining, w)
			continue
		}
		switch res, updated, aerr := advanceRun(ctx, prov, w); res {
		case pollApproved:
			logger.Printf("%s: approved and advanced", label(w))
		case pollAdvanced:
			logger.Printf("%s: %s approved; now awaiting %s", label(w), w.Approver, updated.Approver)
			remaining = append(remaining, updated)
		case pollDeclined:
			logger.Printf("%s: declined; no longer watching", label(w))
		case pollExpired:
			logger.Printf("%s: expired; ran the timeout branch", label(w))
		case pollPending:
			logger.Printf("%s: waiting on approval", label(w))
			remaining = append(remaining, updated)
		case pollFailed:
			logger.Printf("%s: %v", label(w), aerr)
			remaining = append(remaining, updated)
		}
	}
	if err := saveWatches(remaining); err != nil {
		logger.Printf("save watches: %v", err)
	}
}

// label describes a watched hold for logging.
func label(w watchItem) string {
	if w.Subject != "" {
		return fmt.Sprintf("%q (%s)", w.Subject, w.Provider)
	}
	return fmt.Sprintf("%s %s", w.Provider, w.HoldID)
}

// advanceRun advances a watched workflow run: it loads the workflow, checks the
// current gate's approver, and on acceptance runs the steps up to the next gate. When
// the next gate is another approver, it invites them and returns the updated watch
// item so the daemon keeps watching that gate; when the flow completes it returns
// approved. The provider is injected so the logic is testable. It returns the watch
// item to persist, which is unchanged except when the chain advances.
func advanceRun(ctx context.Context, prov calendar.Provider, item watchItem) (pollResult, watchItem, error) {
	wf, err := workflowLoader().Load(item.Workflow)
	if err != nil {
		return pollFailed, item, err
	}
	hold, err := prov.GetHold(ctx, item.HoldID)
	if err != nil {
		return pollFailed, item, err
	}
	notifier := resolveNotifier()
	switch gateResponse(hold, item) {
	case calendar.ResponseAccepted:
		gate, werr := walkSteps(ctx, prov, notifier, item.Provider, wf, wf.Next(item.Step, workflow.OutcomeAccepted), hold)
		if werr != nil {
			return pollFailed, item, werr
		}
		if gate < 0 {
			return pollApproved, item, nil
		}
		// The chain continues: invite the next approver and watch that gate.
		next := calendar.Person{Email: wf.Steps[gate].Approver}
		if _, uerr := prov.UpdateHold(ctx, inviteRequired(hold, next)); uerr != nil {
			return pollFailed, item, fmt.Errorf("invite next approver: %w", uerr)
		}
		item.Step = gate
		item.Approver = next.Email
		item.CreatedAt = time.Now()
		return pollAdvanced, item, nil
	case calendar.ResponseDeclined:
		if item.Step >= 0 && item.Step < len(wf.Steps) {
			if target, ok := wf.Steps[item.Step].On[workflow.OutcomeDeclined]; ok && target != "end" {
				if _, werr := walkSteps(ctx, prov, notifier, item.Provider, wf, wf.StepIndex(target), hold); werr != nil {
					return pollFailed, item, werr
				}
			}
		}
		return pollDeclined, item, nil
	default:
		if item.Step >= 0 && item.Step < len(wf.Steps) {
			step := wf.Steps[item.Step]
			if d := step.ParsedTimeout(); d > 0 && !item.CreatedAt.IsZero() && time.Since(item.CreatedAt) > d {
				if target, ok := step.On[workflow.OutcomeExpired]; ok && target != "end" {
					if _, werr := walkSteps(ctx, prov, notifier, item.Provider, wf, wf.StepIndex(target), hold); werr != nil {
						return pollFailed, item, werr
					}
				}
				return pollExpired, item, nil
			}
		}
		return pollPending, item, nil
	}
}

// gateResponse reports the current gate's approver response. In a chain it checks the
// specific approver the watch item records; for an older single-approver watch it
// falls back to the first required attendee.
func gateResponse(hold calendar.Hold, item watchItem) calendar.Response {
	if item.Approver == "" {
		return managerAttendee(hold).Response
	}
	switch {
	case hold.ApprovedBy(item.Approver):
		return calendar.ResponseAccepted
	case hold.DeclinedBy(item.Approver):
		return calendar.ResponseDeclined
	default:
		return calendar.ResponseNotResponded
	}
}

// inviteRequired returns the hold with person added as a required attendee, skipping a
// duplicate, so the daemon can invite the next approver in a chain.
func inviteRequired(hold calendar.Hold, person calendar.Person) calendar.Hold {
	for _, a := range hold.Attendees {
		if strings.EqualFold(a.Person.Email, person.Email) {
			return hold
		}
	}
	hold.Attendees = append(hold.Attendees, calendar.Attendee{Person: person, Role: calendar.RoleRequired})
	return hold
}
