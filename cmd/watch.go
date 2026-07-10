package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// watchItem is a workflow run the daemon advances when the manager responds.
type watchItem struct {
	// Provider is the calendar provider that owns the hold.
	Provider string `json:"provider"`
	// HoldID is the provider event identifier.
	HoldID string `json:"hold_id"`
	// Workflow is the name of the workflow driving this hold.
	Workflow string `json:"workflow"`
	// Step is the index of the pending step, the approval gate the daemon waits on.
	Step int `json:"step"`
	// Approver is the email the current gate waits on, so the daemon checks the right
	// person in a multi-approver chain. Empty falls back to the first required attendee.
	Approver string `json:"approver,omitempty"`
	// Subject is the hold title, kept for readable logs.
	Subject string `json:"subject,omitempty"`
	// CreatedAt is when the watch was enqueued, used to time an approve step's
	// timeout so the daemon can run the expired branch.
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// watchPath returns the watch-list file location: VAMOOSE_WATCH_FILE when set,
// which the Slack server uses to keep each linked user's watches in their own file,
// otherwise the default under the user config directory.
func watchPath() (string, error) {
	if p := os.Getenv("VAMOOSE_WATCH_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "watches.json"), nil
}

// loadWatches reads the watch list, returning nil when the file is absent.
func loadWatches() ([]watchItem, error) {
	path, err := watchPath()
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
	var items []watchItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// saveWatches writes the watch list, creating parent directories as needed.
func saveWatches(items []watchItem) error {
	path, err := watchPath()
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

// addWatch appends a hold to the watch list, replacing any existing entry with
// the same provider and hold id.
func addWatch(item watchItem) error {
	items, err := loadWatches()
	if err != nil {
		return err
	}
	out := make([]watchItem, 0, len(items)+1)
	for _, w := range items {
		if w.Provider == item.Provider && w.HoldID == item.HoldID {
			continue
		}
		out = append(out, w)
	}
	out = append(out, item)
	return saveWatches(out)
}

// removeWatch drops the watch matching provider and holdID, if present.
func removeWatch(provider, holdID string) error {
	items, err := loadWatches()
	if err != nil {
		return err
	}
	out := make([]watchItem, 0, len(items))
	for _, w := range items {
		if w.Provider == provider && w.HoldID == holdID {
			continue
		}
		out = append(out, w)
	}
	return saveWatches(out)
}
