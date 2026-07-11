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

	"github.com/dcadolph/vamoose/internal/audit"
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
		pollSchedules(ctx, logger)
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
			logger.Printf("%s: advanced to the next step", label(w))
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
	// This end-of-pass save records each hold's final state. Crash-safety across it comes
	// from the per-step checkpoint the daemon writes while a branch runs (see
	// checkpointFor): a crash resumes at the next step rather than replaying the branch. A
	// single step can still repeat if the crash lands between its side effect and its
	// checkpoint write; a transactional store would close that last window.
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

// advanceRun advances a watched workflow run: it loads the workflow and acts on the
// current waiting step. A wait step advances once its delay elapses; an approve step
// advances on the approver's response, or its timeout. Advancing runs the branch the
// gate opens up to the next waiting step, inviting the next approver when the chain
// continues, and returns the watch item to persist, unchanged unless the run advances.
// A branch that fails partway checkpoints its progress on the item so the next poll
// resumes at the failed step rather than repeating completed side effects. The provider
// is injected so the logic is testable.
func advanceRun(ctx context.Context, prov calendar.Provider, item watchItem) (pollResult, watchItem, error) {
	wf, err := workflowLoader().Load(item.Workflow)
	if err != nil {
		return pollFailed, item, err
	}
	// A run whose branch already completed, whose Done marker survived a crash before the
	// drop, is dropped now without replaying the finished branch.
	if item.Done {
		return pollApproved, item, nil
	}
	hold, err := prov.GetHold(ctx, item.HoldID)
	if err != nil {
		return pollFailed, item, err
	}
	if item.Step < 0 || item.Step >= len(wf.Steps) {
		return pollPending, item, nil
	}
	deps := stepDeps{notifier: resolveNotifier(), recorder: resolveRecorder(), checkpoint: checkpointFor(item)}

	// A prior poll ran this branch partway. Earlier steps already took effect, so the
	// branch is committed: resume at the checkpoint and run it to completion, without
	// re-reading the gate, so a response that changed since (a late decline after the team
	// was already notified) cannot switch branches or repeat completed steps.
	if item.Resume > 0 {
		return advanceToNextGate(ctx, prov, deps, wf, item, hold, item.Resume, pollApproved)
	}

	from, onEnd, waiting := branchPlan(wf, item, hold)
	if waiting {
		return pollPending, item, nil
	}
	recordGateOutcome(ctx, deps.recorder, wf, item, hold, onEnd)
	// Commit to the branch durably before running it, so a crash resumes here rather than
	// re-evaluating the gate and recording the outcome a second time.
	deps.checkpointAt(from)
	return advanceToNextGate(ctx, prov, deps, wf, item, hold, from, onEnd)
}

// checkpointFor returns a function that durably records a watched hold's branch progress
// after each step: resume is the next step to run, or negative when the branch has
// completed, which marks the hold done so it is dropped on the next poll. Writing right
// away means a daemon crash resumes at the next step rather than replaying side effects.
func checkpointFor(item watchItem) func(resume int) {
	return func(resume int) {
		it := item
		if resume < 0 {
			it.Done = true
			it.Resume = 0
		} else {
			it.Resume = resume
		}
		if err := addWatch(it); err != nil {
			fmt.Fprintf(os.Stderr, "vamoose daemon: checkpoint failed: %v\n", err)
		}
	}
}

// recordGateOutcome records the audit event for a gate that just resolved: an approval,
// decline, or expiry, naming the approver. A wait step elapsing is not an approval, so it
// records nothing.
func recordGateOutcome(ctx context.Context, rec audit.Recorder, wf workflow.Workflow, item watchItem, hold calendar.Hold, onEnd pollResult) {
	if wf.Steps[item.Step].Verb == workflow.VerbWait {
		return
	}
	var action audit.Action
	switch onEnd {
	case pollDeclined:
		action = audit.ActionDeclined
	case pollExpired:
		action = audit.ActionExpired
	default:
		action = audit.ActionApproved
	}
	recordAudit(ctx, rec, audit.Event{
		Workflow: wf.Name, Provider: item.Provider, HoldID: item.HoldID,
		Action: action, Actor: gateActor(hold, item),
	})
}

// gateActor returns the email of the approver the current gate waits on, for the audit
// record: the chain approver the item names, or the first required attendee.
func gateActor(hold calendar.Hold, item watchItem) string {
	if item.Approver != "" {
		return item.Approver
	}
	return managerAttendee(hold).Person.Email
}

// branchPlan decides which branch the pending gate opens and how completing it reads.
// It returns the branch's first step, the poll result to report when the branch runs to
// the end (approved for a wait or acceptance, declined or expired for those outcomes),
// and whether the gate is still waiting, meaning there is no branch to run yet. A wait
// step opens once its delay passes; an approve step opens on the approver's accept or
// decline, or on its timeout. An accept with no explicit branch falls through to the next
// step; a decline or expiry with no branch runs nothing and simply ends.
func branchPlan(wf workflow.Workflow, item watchItem, hold calendar.Hold) (from int, onEnd pollResult, waiting bool) {
	pending := wf.Steps[item.Step]
	if pending.Verb == workflow.VerbWait {
		if d := pending.ParsedFor(); d <= 0 || item.CreatedAt.IsZero() || time.Since(item.CreatedAt) < d {
			return -1, pollPending, true
		}
		return wf.Next(item.Step, ""), pollApproved, false
	}
	switch gateResponse(hold, item) {
	case calendar.ResponseAccepted:
		return wf.Next(item.Step, workflow.OutcomeAccepted), pollApproved, false
	case calendar.ResponseDeclined:
		return branchStart(wf, pending, workflow.OutcomeDeclined), pollDeclined, false
	default:
		if d := pending.ParsedTimeout(); d > 0 && !item.CreatedAt.IsZero() && time.Since(item.CreatedAt) > d {
			return branchStart(wf, pending, workflow.OutcomeExpired), pollExpired, false
		}
		return -1, pollPending, true
	}
}

// branchStart returns the first step of an approve step's named outcome branch, or -1
// when the branch is absent or ends at once, in which case the outcome completes with no
// steps to run. Unlike an accepted outcome, a decline or expiry does not fall through to
// the following step.
func branchStart(wf workflow.Workflow, step workflow.Step, outcome string) int {
	target, ok := step.On[outcome]
	if !ok || target == "end" {
		return -1
	}
	return wf.StepIndex(target)
}

// advanceToNextGate runs the branch forward from `from` to the next waiting step or the
// end, checkpointing progress on the item so a transient failure resumes at the failed
// step instead of restarting. When the next step is a named chain approver it invites
// them; a wait step or a manager gate needs no invite. It returns the item to persist:
// advanced at a new gate, the branch's end result (onEnd) when the flow completes, or
// failed with the resume checkpoint set when a step errors.
func advanceToNextGate(ctx context.Context, prov calendar.Provider, deps stepDeps, wf workflow.Workflow, item watchItem, hold calendar.Hold, from int, onEnd pollResult) (pollResult, watchItem, error) {
	gate, err := walkSteps(ctx, prov, deps, item.Provider, wf, from, hold)
	if err != nil {
		item.Resume = gate
		return pollFailed, item, err
	}
	item.Resume = 0
	if gate < 0 {
		return onEnd, item, nil
	}
	next := wf.Steps[gate]
	item.Step = gate
	item.CreatedAt = time.Now()
	if next.Verb == workflow.VerbApprove && next.Approver != "" {
		if _, uerr := prov.UpdateHold(ctx, inviteRequired(hold, calendar.Person{Email: next.Approver})); uerr != nil {
			return pollFailed, item, fmt.Errorf("invite next approver: %w", uerr)
		}
		item.Approver = next.Approver
	} else {
		item.Approver = ""
	}
	recordAudit(ctx, deps.recorder, audit.Event{
		Workflow: wf.Name, Provider: item.Provider, HoldID: item.HoldID,
		Action: audit.ActionAdvanced, Detail: item.Approver,
	})
	return pollAdvanced, item, nil
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
