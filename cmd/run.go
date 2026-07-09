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

	"github.com/dcadolph/vamoose/internal/calendar"
	"github.com/dcadolph/vamoose/internal/workflow"
)

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
			mgr, merr := resolveManager(ctx, prov, opt.Manager)
			if merr != nil {
				return merr
			}
			hold.Attendees = []calendar.Attendee{{Person: mgr, Role: calendar.RoleRequired}}
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
	fmt.Fprintf(os.Stdout, "Started %q. Hold id: %s\n", wf.Name, created.ID)

	return runSteps(ctx, prov, providerName, wf, 1, created, opt.Watch)
}

// runSteps runs the workflow steps from index `from` against an existing hold.
// Notify fans out to the team, cancel deletes the hold, and an approval gate stops
// execution, recording the run for the daemon when watching. The run command calls
// it after creation; the daemon calls it again past the gate once approval lands.
func runSteps(ctx context.Context, prov calendar.Provider, providerName string, wf workflow.Workflow, from int, hold calendar.Hold, watch bool) error {
	for i := from; i < len(wf.Steps); i++ {
		switch wf.Steps[i].Verb {
		case workflow.VerbApprove:
			return gateOnApproval(providerName, wf, i, hold, watch)
		case workflow.VerbNotify:
			if err := promoteHold(ctx, prov, hold); err != nil {
				return err
			}
		case workflow.VerbCancel:
			if err := prov.DeleteHold(ctx, hold.ID); err != nil {
				return fmt.Errorf("cancel hold: %w", err)
			}
			forgetHold(holdRef{Provider: providerName, ID: hold.ID})
			fmt.Fprintf(os.Stdout, "Canceled hold %s.\n", hold.ID)
			return nil
		}
	}
	return nil
}

// gateOnApproval stops the workflow at its approval step. When watching, it records
// the run at that step so the daemon advances it once the manager accepts.
func gateOnApproval(providerName string, wf workflow.Workflow, stepIdx int, hold calendar.Hold, watch bool) error {
	if !watch {
		fmt.Fprintf(os.Stdout, "Waiting on approval. Check with 'vamoose check' or run 'vamoose daemon' after 'vamoose run %s --watch'.\n", wf.Name)
		return nil
	}
	if err := addWatch(watchItem{
		Provider: providerName,
		HoldID:   hold.ID,
		Workflow: wf.Name,
		Step:     stepIdx,
		Subject:  hold.Subject,
	}); err != nil {
		return fmt.Errorf("add watch: %w", err)
	}
	fmt.Fprintln(os.Stdout, "Watching for approval. Run 'vamoose daemon' to advance the workflow when approved.")
	return nil
}

// firstApproveStep returns the index of the workflow's approval step, or -1 when it
// has none.
func firstApproveStep(wf workflow.Workflow) int {
	for i, s := range wf.Steps {
		if s.Verb == workflow.VerbApprove {
			return i
		}
	}
	return -1
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

// previewWorkflow prints the plan a dry run would carry out.
func previewWorkflow(w io.Writer, wf workflow.Workflow, hold calendar.Hold) {
	fmt.Fprintf(w, "Workflow %q (dry run)\n", wf.Name)
	fmt.Fprintf(w, "  create %q as %s, %s -> %s\n",
		hold.Subject, hold.ShowAs, hold.Start.Format(time.RFC3339), hold.End.Format(time.RFC3339))
	for _, a := range hold.Attendees {
		fmt.Fprintf(w, "  invite %s (%s)\n", personLabel(a.Person), a.Role)
	}
	for _, s := range wf.Steps[1:] {
		fmt.Fprintf(w, "  then %s\n", s.Verb)
	}
}
