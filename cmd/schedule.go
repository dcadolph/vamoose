package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// scheduleItem is a workflow the daemon reruns on an interval.
type scheduleItem struct {
	// Workflow is the name of the workflow to run.
	Workflow string `json:"workflow"`
	// Provider is the calendar provider to run against, empty for the default.
	Provider string `json:"provider,omitempty"`
	// Every is the interval between runs, as a Go duration such as "168h".
	Every string `json:"every"`
	// NextRun is when the daemon should next run the workflow.
	NextRun time.Time `json:"next_run"`
	// Phrase is the relative date window resolved at each run, such as "next week".
	Phrase string `json:"phrase,omitempty"`
	// Subject is the event subject passed to the run.
	Subject string `json:"subject,omitempty"`
	// Manager is the approver email passed to the run, for a directory-less backend.
	Manager string `json:"manager,omitempty"`
}

// ParsedEvery returns the interval as a duration, or zero when unset or unparseable.
// Validation rejects an unparseable interval, so a nonzero return means the daemon
// reruns the workflow that often.
func (s scheduleItem) ParsedEvery() time.Duration {
	d, err := time.ParseDuration(s.Every)
	if err != nil {
		return 0
	}
	return d
}

// schedulePath returns the schedule file location under the user config directory.
func schedulePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "schedules.json"), nil
}

// loadSchedules reads the schedule list, returning nil when the file is absent.
func loadSchedules() ([]scheduleItem, error) {
	path, err := schedulePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []scheduleItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// saveSchedules writes the schedule list, creating parent directories as needed.
func saveSchedules(items []scheduleItem) error {
	path, err := schedulePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// addSchedule appends a schedule to the list.
func addSchedule(item scheduleItem) error {
	items, err := loadSchedules()
	if err != nil {
		return err
	}
	return saveSchedules(append(items, item))
}

// runSchedule dispatches the schedule subcommands: add, list, and remove.
func runSchedule(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("schedule: use add, list, or remove")
	}
	switch args[0] {
	case "add":
		return scheduleAdd(ctx, args[1:])
	case "list":
		return scheduleList()
	case "remove", "rm":
		return scheduleRemove(args[1:])
	default:
		return fmt.Errorf("schedule: unknown subcommand %q; use add, list, or remove", args[0])
	}
}

// scheduleAdd records a workflow to rerun on an interval. The daemon fires it.
func scheduleAdd(_ context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("schedule add: name a workflow to schedule")
	}
	name := args[0]
	fs := flag.NewFlagSet("schedule add", flag.ContinueOnError)
	var (
		every    = fs.String("every", "", "Interval between runs, e.g. 168h (required)")
		phrase   = fs.String("phrase", "", "Relative date window resolved each run, e.g. \"next week\" (required)")
		subject  = fs.String("subject", "", "Event subject")
		manager  = fs.String("manager", "", "Approver email for a directory-less backend")
		provider = fs.String("provider", "", "Calendar provider; overrides VAMOOSE_PROVIDER")
	)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if _, err := workflowLoader().Load(name); err != nil {
		return fmt.Errorf("schedule add: %w", err)
	}
	if d, err := time.ParseDuration(*every); err != nil || d <= 0 {
		return fmt.Errorf("schedule add: --every must be a positive duration such as 168h")
	}
	if *phrase == "" {
		return fmt.Errorf("schedule add: --phrase is required, the date window to run for, e.g. \"next week\"")
	}
	item := scheduleItem{
		Workflow: name, Provider: *provider, Every: *every,
		NextRun: time.Now(), Phrase: *phrase, Subject: *subject, Manager: *manager,
	}
	if err := addSchedule(item); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Scheduled %q every %s for %q. Run 'vamoose daemon' to fire it.\n", name, *every, *phrase)
	return nil
}

// scheduleList prints the current schedules with their index for removal.
func scheduleList() error {
	items, err := loadSchedules()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stdout, "No schedules. Add one with 'vamoose schedule add <workflow> --every <dur> --phrase <window>'.")
		return nil
	}
	for i, s := range items {
		fmt.Fprintf(os.Stdout, "%d  %s  every %s  next %s  %q\n",
			i, s.Workflow, s.Every, s.NextRun.Format(time.RFC3339), s.Phrase)
	}
	return nil
}

// scheduleRemove drops the schedule at the given index from schedule list.
func scheduleRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("schedule remove: give the index from 'schedule list'")
	}
	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("schedule remove: %q is not an index", args[0])
	}
	items, err := loadSchedules()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(items) {
		return fmt.Errorf("schedule remove: no schedule at index %d", idx)
	}
	removed := items[idx].Workflow
	items = append(items[:idx], items[idx+1:]...)
	if err := saveSchedules(items); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "Removed schedule %d (%s).\n", idx, removed)
	return nil
}

// scheduleRunner runs a due schedule. It is injected so the daemon loop is testable.
type scheduleRunner func(ctx context.Context, s scheduleItem) error

// fireSchedules runs every schedule whose next run has passed and advances each to its
// next interval, returning the updated list to persist. A run error is logged and the
// schedule still advances, so a failing run does not retry every poll or catch up on
// missed intervals after downtime.
func fireSchedules(ctx context.Context, now time.Time, schedules []scheduleItem, run scheduleRunner, logger *log.Logger) []scheduleItem {
	out := make([]scheduleItem, 0, len(schedules))
	for _, s := range schedules {
		if !s.NextRun.IsZero() && !s.NextRun.After(now) {
			if err := run(ctx, s); err != nil {
				logger.Printf("schedule %q: %v", s.Workflow, err)
			} else {
				logger.Printf("schedule %q: ran", s.Workflow)
			}
			if d := s.ParsedEvery(); d > 0 {
				s.NextRun = now.Add(d)
			}
		}
		out = append(out, s)
	}
	return out
}
