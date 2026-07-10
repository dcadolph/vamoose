package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
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
