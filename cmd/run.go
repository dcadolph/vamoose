package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dcadolph/vamoose/internal/audit"
	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/comms"
	"github.com/dcadolph/vamoose/internal/hris"
	"github.com/dcadolph/vamoose/internal/workflow"
)

// stepDeps carries the side-effect adapters the executor uses: the comms notifier for
// message steps, the audit recorder for run history, and the checkpoint that persists
// branch progress. Any may be nil, in which case that effect is skipped.
type stepDeps struct {
	// notifier posts message steps to a comms channel.
	notifier comms.Notifier
	// recorder appends run-history events.
	recorder audit.Recorder
	// checkpoint durably records how far the branch has run: resume is the next step to
	// run, or negative when the branch has completed. The daemon sets it so a crash
	// resumes rather than replaying; the run command leaves it nil.
	checkpoint func(resume int)
	// filer files approved leave with an HR system for a leave step; nil when no HR
	// system is configured.
	filer hris.Filer
}

// checkpointAt persists the branch's progress when a checkpoint is set, so a daemon crash
// resumes at `resume` (or drops the run when resume is negative) instead of replaying.
func (d stepDeps) checkpointAt(resume int) {
	if d.checkpoint != nil {
		d.checkpoint(resume)
	}
}

// resolveNotifier returns the configured comms notifier, or nil when none is set. A
// Slack bot token (VAMOOSE_SLACK_BOT_TOKEN) enables Slack, and SMTP settings
// (VAMOOSE_SMTP_HOST and friends) enable email. When both are set, a message routes by
// its channel: an address goes to email, anything else to Slack.
func resolveNotifier() comms.Notifier {
	var slack, email comms.Notifier
	if token := os.Getenv("VAMOOSE_SLACK_BOT_TOKEN"); token != "" {
		slack = comms.NewSlackNotifier(token)
	}
	if host := os.Getenv("VAMOOSE_SMTP_HOST"); host != "" {
		email = comms.NewEmailNotifier(comms.EmailConfig{
			Host:     host,
			Port:     os.Getenv("VAMOOSE_SMTP_PORT"),
			Username: os.Getenv("VAMOOSE_SMTP_USERNAME"),
			Password: os.Getenv("VAMOOSE_SMTP_PASSWORD"),
			From:     os.Getenv("VAMOOSE_SMTP_FROM"),
		})
	}
	if slack == nil && email == nil {
		return nil
	}
	return comms.Route(slack, email)
}

// resolveFiler returns the configured HR leave filer, or nil when none is set. BambooHR
// (VAMOOSE_BAMBOOHR_SUBDOMAIN and VAMOOSE_BAMBOOHR_API_KEY) is preferred; otherwise a
// generic webhook (VAMOOSE_LEAVE_WEBHOOK_URL) posts the leave anywhere, so any HR system
// or automation platform can receive it.
func resolveFiler() hris.Filer {
	if sub, key := os.Getenv("VAMOOSE_BAMBOOHR_SUBDOMAIN"), os.Getenv("VAMOOSE_BAMBOOHR_API_KEY"); sub != "" && key != "" {
		return hris.NewBambooHRFiler(sub, key, os.Getenv("VAMOOSE_BAMBOOHR_STATUS"))
	}
	if url := os.Getenv("VAMOOSE_LEAVE_WEBHOOK_URL"); url != "" {
		return hris.NewWebhookFiler(url, os.Getenv("VAMOOSE_LEAVE_WEBHOOK_AUTH"))
	}
	return nil
}

// workflowsDir returns the user workflow directory under the config directory.
func workflowsDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "workflows"), nil
}

// workflowLoader returns a loader that prefers user workflows over built-ins.
func workflowLoader() workflow.Loader {
	dir, err := workflowsDir()
	if err != nil {
		return workflow.Loader{}
	}
	return workflow.Loader{UserDir: dir}
}

// runOptions carries the resolved inputs for executing a workflow.
type runOptions struct {
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
	// Attendees is the comma-separated invite list for an event workflow.
	Attendees string
	// Start is the window start.
	Start time.Time
	// End is the window end, exclusive for all-day windows.
	End time.Time
	// AllDay marks a full-day window.
	AllDay bool
	// Watch enqueues the hold for the daemon to advance on approval.
	Watch bool
	// DryRun prints the plan without calling the calendar.
	DryRun bool
}

// runRun executes a named workflow: it creates the hold its first step defines,
// runs the immediate steps that follow, and stops at an approval step.
func runRun(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("run: name a workflow to run (see vamoose workflows)")
	}
	name := args[0]
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var (
		start     = fs.String("start", "", "Start date/time, RFC3339 or YYYY-MM-DD; overrides the phrase")
		end       = fs.String("end", "", "End date/time; overrides the phrase")
		subject   = fs.String("subject", "", "Event subject; defaults to the workflow default")
		body      = fs.String("body", "", "Event description")
		manager   = fs.String("manager", "", "Manager email for approval; resolved from the directory when empty")
		attendees = fs.String("attendees", "", "Comma-separated attendees for an event workflow")
		provider  = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER (default graph)")
		tzFlag    = fs.String("tz", "", "IANA time zone for event times")
		watch     = fs.Bool("watch", false, "Watch for approval so the daemon advances the workflow")
		dryRun    = fs.Bool("dry-run", false, "Print what would happen without calling the calendar")
	)
	phraseWords, flagArgs := splitPhrase(args[1:])
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	phrase := strings.Join(phraseWords, " ")
	if phrase == "" && fs.NArg() > 0 {
		phrase = strings.Join(fs.Args(), " ")
	}

	wf, err := workflowLoader().Load(name)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	startAt, endAt, allDay, err := resolveWindow(*start, *end, phrase)
	if err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}

	subj := *subject
	if subj == "" {
		subj = defaultSubject(wf)
	}
	return executeWorkflow(ctx, wf, runOptions{
		Provider: *provider, TZ: *tzFlag, Subject: subj, Body: *body,
		Manager: *manager, Attendees: *attendees,
		Start: startAt, End: endAt, AllDay: allDay,
		Watch: *watch, DryRun: *dryRun,
	})
}

// executeWorkflow builds the provider and runs the workflow against it.
func executeWorkflow(ctx context.Context, wf workflow.Workflow, opt runOptions) error {
	providerName := resolveProvider(opt.Provider)
	prov, err := newProvider(providerName, resolveTimeZone(opt.TZ))
	if err != nil {
		return err
	}
	return runWorkflowOn(ctx, prov, providerName, wf, opt)
}

// runWorkflowOn creates the hold the workflow's first step defines against prov,
// then runs the immediate steps until an approval gate. The provider is injected
// so the logic is testable.
func runWorkflowOn(ctx context.Context, prov calendar.Provider, providerName string, wf workflow.Workflow, opt runOptions) error {
	create := wf.Steps[0]
	hold := calendar.Hold{
		Subject: opt.Subject,
		Body:    opt.Body,
		Start:   opt.Start,
		End:     opt.End,
		AllDay:  opt.AllDay,
		ShowAs:  createShowAs(create),
	}

	switch create.Verb {
	case workflow.VerbHold:
		if wf.Has(workflow.VerbApprove) {
			approver, merr := resolveApprover(ctx, prov, firstApproveStep(wf), opt.Manager)
			if merr != nil {
				return merr
			}
			hold.Attendees = []calendar.Attendee{{Person: approver, Role: calendar.RoleRequired}}
		}
	case workflow.VerbEvent:
		if opt.Subject == "" {
			return fmt.Errorf("run %s: an event workflow needs --subject", wf.Name)
		}
		hold.Attendees = attendeesFromCSV(opt.Attendees)
	}

	if opt.DryRun {
		previewWorkflow(os.Stdout, wf, hold)
		return nil
	}

	created, err := prov.CreateHold(ctx, hold)
	if err != nil {
		return fmt.Errorf("create hold: %w", err)
	}
	if err := saveState(state{LastHold: holdRef{Provider: providerName, ID: created.ID}}); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	fmt.Fprintln(os.Stdout, startedMessage(wf, created))

	deps := stepDeps{notifier: resolveNotifier(), recorder: resolveRecorder(), filer: resolveFiler()}
	recordAudit(ctx, deps.recorder, audit.Event{
		Workflow: wf.Name, Provider: providerName, HoldID: created.ID, Action: audit.ActionCreated,
	})
	return runSteps(ctx, prov, deps, providerName, wf, wf.Next(0, ""), created, opt.Watch)
}

// runSteps walks the workflow from `from` for the run command: it runs the immediate
// steps and, when it reaches an approve gate, records the run for the daemon if
// watching. The daemon calls walkSteps directly to advance a chain gate to gate.
func runSteps(ctx context.Context, prov calendar.Provider, deps stepDeps, providerName string, wf workflow.Workflow, from int, hold calendar.Hold, watch bool) error {
	gate, err := walkSteps(ctx, prov, deps, providerName, wf, from, hold)
	if err != nil {
		return err
	}
	if gate >= 0 {
		return gateWaiting(providerName, wf, gate, hold, watch)
	}
	return nil
}

// walkSteps runs the workflow's immediate steps from `from` against an existing hold,
// following each step's next or fall-through, until it reaches a waiting step (an
// approve gate or a wait delay) or the end. Notify fans out to the team, note marks the
// requester's calendar, message posts to a channel, and cancel deletes the hold. A step
// whose when guard denies is skipped. On success it returns the index of the waiting
// step it stopped at, or -1 when the flow ran to completion, so a caller can gate the
// run or advance the next step. On error it returns the index of the step that failed,
// so the daemon can checkpoint and resume there rather than repeating completed steps.
func walkSteps(ctx context.Context, prov calendar.Provider, deps stepDeps, providerName string, wf workflow.Workflow, from int, hold calendar.Hold) (int, error) {
	visited := make(map[int]bool)
	for i := from; i >= 0 && i < len(wf.Steps); i = wf.Next(i, "") {
		if visited[i] {
			return -1, nil
		}
		visited[i] = true
		if !wf.Steps[i].When.Allows(time.Now(), len(hold.Attendees)) {
			fmt.Fprintf(os.Stderr, "vamoose: skipping step %d (%s): its when guard does not allow now\n", i, wf.Steps[i].Verb)
			continue
		}
		if wf.Steps[i].Verb.Waits() {
			return i, nil
		}
		switch wf.Steps[i].Verb {
		case workflow.VerbNotify:
			if err := promoteHold(ctx, prov, hold); err != nil {
				return i, err
			}
			recordAudit(ctx, deps.recorder, stepEvent(wf, providerName, hold, audit.ActionNotified, ""))
			deps.checkpointAt(wf.Next(i, ""))
		case workflow.VerbNote:
			if err := createNote(ctx, prov, wf.Steps[i], hold); err != nil {
				return i, err
			}
			recordAudit(ctx, deps.recorder, stepEvent(wf, providerName, hold, audit.ActionNoted, wf.Steps[i].Subject))
			deps.checkpointAt(wf.Next(i, ""))
		case workflow.VerbMessage:
			if err := sendMessage(ctx, deps.notifier, wf.Steps[i], hold); err != nil {
				return i, err
			}
			recordAudit(ctx, deps.recorder, stepEvent(wf, providerName, hold, audit.ActionMessaged, wf.Steps[i].Channel))
			deps.checkpointAt(wf.Next(i, ""))
		case workflow.VerbLeave:
			if err := fileLeave(ctx, deps.filer, wf.Steps[i], hold); err != nil {
				return i, err
			}
			recordAudit(ctx, deps.recorder, stepEvent(wf, providerName, hold, audit.ActionFiled, wf.Steps[i].Subject))
			deps.checkpointAt(wf.Next(i, ""))
		case workflow.VerbCancel:
			if err := prov.DeleteHold(ctx, hold.ID); err != nil {
				return i, fmt.Errorf("cancel hold: %w", err)
			}
			forgetHold(holdRef{Provider: providerName, ID: hold.ID})
			recordAudit(ctx, deps.recorder, stepEvent(wf, providerName, hold, audit.ActionCanceled, ""))
			deps.checkpointAt(-1)
			fmt.Fprintf(os.Stdout, "Canceled hold %s.\n", hold.ID)
			return -1, nil
		}
	}
	return -1, nil
}

// stepEvent builds an audit event for a workflow side-effect step.
func stepEvent(wf workflow.Workflow, providerName string, hold calendar.Hold, action audit.Action, detail string) audit.Event {
	return audit.Event{
		Workflow: wf.Name,
		Provider: providerName,
		HoldID:   hold.ID,
		Action:   action,
		Detail:   detail,
	}
}

// createNote creates a short informational event on the requester's own calendar,
// marking an outcome such as a decline. It reuses the hold's window.
func createNote(ctx context.Context, prov calendar.Provider, step workflow.Step, hold calendar.Hold) error {
	subject := step.Subject
	if subject == "" {
		subject = "Time off update"
	}
	created, err := prov.CreateHold(ctx, calendar.Hold{
		Subject: subject,
		Start:   hold.Start,
		End:     hold.End,
		AllDay:  hold.AllDay,
		ShowAs:  calendar.ShowFree,
	})
	if err != nil {
		return fmt.Errorf("create note: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Noted on your calendar: %q (id: %s)\n", subject, created.ID)
	return nil
}

// sendMessage posts a message step's text to its channel through the notifier. The
// text is the step subject, falling back to the hold subject, so the message carries
// the run's context without the workflow hardcoding it.
func sendMessage(ctx context.Context, notifier comms.Notifier, step workflow.Step, hold calendar.Hold) error {
	if notifier == nil {
		return fmt.Errorf("message step needs a comms backend: set VAMOOSE_SLACK_BOT_TOKEN or VAMOOSE_SMTP_HOST")
	}
	text := step.Subject
	if text == "" {
		text = hold.Subject
	}
	if text == "" {
		text = "Calendar update"
	}
	if err := notifier.Notify(ctx, step.Channel, text); err != nil {
		return fmt.Errorf("message: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Messaged %s.\n", step.Channel)
	return nil
}

// fileLeave files the hold's dates as leave with the configured HR system. The employee
// and time-off type come from the environment; the note is the step subject, falling back
// to the hold subject.
func fileLeave(ctx context.Context, filer hris.Filer, step workflow.Step, hold calendar.Hold) error {
	if filer == nil {
		return fmt.Errorf("leave step needs an HR system: set VAMOOSE_BAMBOOHR_* or VAMOOSE_LEAVE_WEBHOOK_URL")
	}
	note := step.Subject
	if note == "" {
		note = hold.Subject
	}
	id, err := filer.FileLeave(ctx, hris.Leave{
		EmployeeID: os.Getenv("VAMOOSE_BAMBOOHR_EMPLOYEE_ID"),
		TypeID:     os.Getenv("VAMOOSE_BAMBOOHR_TYPE_ID"),
		Start:      hold.Start,
		End:        hold.End,
		Note:       note,
	})
	if err != nil {
		return fmt.Errorf("file leave: %w", err)
	}
	fmt.Fprintf(os.Stdout, "Filed leave with the HR system (request %s).\n", id)
	return nil
}

// firstApproveStep returns the workflow's first approve step, or a zero step when it
// has none. It picks the approver invited when the hold is created.
func firstApproveStep(wf workflow.Workflow) workflow.Step {
	for _, s := range wf.Steps {
		if s.Verb == workflow.VerbApprove {
			return s
		}
	}
	return workflow.Step{}
}

// resolveApprover returns the person an approve step waits on: the step's explicit
// approver email, or the directory manager (or --manager) when the step names none.
func resolveApprover(ctx context.Context, prov calendar.Provider, step workflow.Step, managerFlag string) (calendar.Person, error) {
	if step.Approver != "" {
		return calendar.Person{Email: step.Approver}, nil
	}
	return resolveManager(ctx, prov, managerFlag)
}

// gateWaiting stops the workflow at a waiting step, an approval or a wait delay. When
// watching, it records the run at that step, with the approver for an approve gate, so
// the daemon advances it once the approver accepts or the delay passes.
func gateWaiting(providerName string, wf workflow.Workflow, stepIdx int, hold calendar.Hold, watch bool) error {
	step := wf.Steps[stepIdx]
	isWait := step.Verb == workflow.VerbWait
	if !watch {
		if isWait {
			fmt.Fprintln(os.Stdout, "This workflow pauses here. Add --watch and run 'vamoose daemon' to advance it after the delay.")
		} else {
			fmt.Fprintln(os.Stdout, "Waiting on approval. Run 'vamoose check' to poll, or add --watch and run 'vamoose daemon' to advance on approval.")
		}
		return nil
	}
	item := watchItem{
		Provider:  providerName,
		HoldID:    hold.ID,
		Workflow:  wf.Name,
		Step:      stepIdx,
		Subject:   hold.Subject,
		CreatedAt: time.Now(),
	}
	if !isWait {
		item.Approver = managerAttendee(hold).Person.Email
	}
	if err := addWatch(item); err != nil {
		return fmt.Errorf("add watch: %w", err)
	}
	if isWait {
		fmt.Fprintf(os.Stdout, "Waiting %s. Run 'vamoose daemon' to advance the workflow after the delay.\n", step.For)
	} else {
		fmt.Fprintln(os.Stdout, "Watching for approval. Run 'vamoose daemon' to advance the workflow when approved.")
	}
	return nil
}

// createShowAs returns the free/busy status for a creating step, applying the verb
// default when the step does not set one.
func createShowAs(s workflow.Step) calendar.ShowAs {
	if s.ShowAs != "" {
		return s.ShowAs
	}
	switch s.Verb {
	case workflow.VerbAway:
		return calendar.ShowOOF
	case workflow.VerbEvent:
		return calendar.ShowBusy
	default:
		return calendar.ShowFree
	}
}

// defaultSubject returns the default event subject for a workflow. Event workflows
// have none, so the subject is required.
func defaultSubject(wf workflow.Workflow) string {
	if wf.Steps[0].Verb == workflow.VerbEvent {
		return ""
	}
	return "Out of office"
}

// startedMessage summarizes a created hold by the action its workflow took, so the
// line reads naturally whether the user ran a workflow by name or through request,
// off, or away.
func startedMessage(wf workflow.Workflow, hold calendar.Hold) string {
	switch wf.Steps[0].Verb {
	case workflow.VerbAway:
		return fmt.Sprintf("Marked out of office. Hold id: %s", hold.ID)
	case workflow.VerbEvent:
		return fmt.Sprintf("Event created. Id: %s", hold.ID)
	default:
		if wf.Has(workflow.VerbApprove) {
			return fmt.Sprintf("Hold created and sent to %s for approval. Hold id: %s",
				managerAttendee(hold).Person.Email, hold.ID)
		}
		return fmt.Sprintf("Hold created. Hold id: %s", hold.ID)
	}
}

// previewWorkflow prints the plan a dry run would carry out.
func previewWorkflow(w io.Writer, wf workflow.Workflow, hold calendar.Hold) {
	fmt.Fprintf(w, "Workflow %q (dry run)\n", wf.Name)
	fmt.Fprintf(w, "  create %q as %s, %s -> %s\n",
		hold.Subject, hold.ShowAs, hold.Start.Format(time.RFC3339), hold.End.Format(time.RFC3339))
	for _, a := range hold.Attendees {
		fmt.Fprintf(w, "  invite %s (%s)\n", personLabel(a.Person), a.Role)
	}
	for _, s := range wf.Steps[1:] {
		if s.Verb == workflow.VerbApprove && len(s.On) > 0 {
			fmt.Fprintf(w, "  approve, then %s%s\n", branchSummary(s), whenSummary(s.When))
			continue
		}
		fmt.Fprintf(w, "  %s%s%s%s\n", stepLabel(s), messageTarget(s), waitFor(s), whenSummary(s.When))
	}
}

// messageTarget describes a message step's channel for the dry-run preview.
func messageTarget(s workflow.Step) string {
	if s.Verb == workflow.VerbMessage && s.Channel != "" {
		return " -> " + s.Channel
	}
	return ""
}

// waitFor describes a wait step's delay for the dry-run preview.
func waitFor(s workflow.Step) string {
	if s.Verb == workflow.VerbWait && s.For != "" {
		return " for " + s.For
	}
	return ""
}

// whenSummary describes a step's guard for the dry-run preview, returning the empty
// string when the guard is unset.
func whenSummary(w workflow.When) string {
	var parts []string
	if w.DayOfWeek != "" {
		parts = append(parts, w.DayOfWeek)
	}
	switch {
	case w.MinAttendees > 0 && w.MaxAttendees > 0:
		parts = append(parts, fmt.Sprintf("%d-%d attendees", w.MinAttendees, w.MaxAttendees))
	case w.MinAttendees > 0:
		parts = append(parts, fmt.Sprintf("%d+ attendees", w.MinAttendees))
	case w.MaxAttendees > 0:
		parts = append(parts, fmt.Sprintf("up to %d attendees", w.MaxAttendees))
	}
	if len(parts) == 0 {
		return ""
	}
	return " when " + strings.Join(parts, ", ")
}

// branchSummary describes an approve step's accepted and declined branches.
func branchSummary(s workflow.Step) string {
	var parts []string
	if t, ok := s.On[workflow.OutcomeAccepted]; ok {
		parts = append(parts, "accepted -> "+t)
	}
	if t, ok := s.On[workflow.OutcomeDeclined]; ok {
		parts = append(parts, "declined -> "+t)
	}
	return strings.Join(parts, ", ")
}

// stepLabel renders a step as its id and verb, or just the verb when they match.
func stepLabel(s workflow.Step) string {
	if s.ID != "" && s.ID != string(s.Verb) {
		return s.ID + " (" + string(s.Verb) + ")"
	}
	return string(s.Verb)
}
