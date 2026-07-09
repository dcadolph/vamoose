package cmd

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	// pollApproved means the manager accepted; the hold was promoted if configured.
	pollApproved
	// pollDeclined means the manager declined.
	pollDeclined
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
		switch res, aerr := advanceRun(ctx, prov, w); res {
		case pollApproved:
			logger.Printf("%s: approved and advanced", label(w))
		case pollDeclined:
			logger.Printf("%s: declined; no longer watching", label(w))
		case pollPending:
			logger.Printf("%s: waiting on the manager", label(w))
			remaining = append(remaining, w)
		case pollFailed:
			logger.Printf("%s: %v", label(w), aerr)
			remaining = append(remaining, w)
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
// manager's response, and on approval runs the steps past the gate (notify and any
// other steps that follow it). The provider is injected so the logic is testable.
func advanceRun(ctx context.Context, prov calendar.Provider, item watchItem) (pollResult, error) {
	wf, err := workflowLoader().Load(item.Workflow)
	if err != nil {
		return pollFailed, err
	}
	hold, err := prov.GetHold(ctx, item.HoldID)
	if err != nil {
		return pollFailed, err
	}
	switch managerAttendee(hold).Response {
	case calendar.ResponseAccepted:
		if err := runSteps(ctx, prov, item.Provider, wf, wf.Next(item.Step, workflow.OutcomeAccepted), hold, false); err != nil {
			return pollFailed, err
		}
		return pollApproved, nil
	case calendar.ResponseDeclined:
		if item.Step >= 0 && item.Step < len(wf.Steps) {
			if target, ok := wf.Steps[item.Step].On[workflow.OutcomeDeclined]; ok && target != "end" {
				if err := runSteps(ctx, prov, item.Provider, wf, wf.StepIndex(target), hold, false); err != nil {
					return pollFailed, err
				}
			}
		}
		return pollDeclined, nil
	default:
		return pollPending, nil
	}
}
