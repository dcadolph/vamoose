package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// state holds cross-invocation vamoose data, currently the last hold id.
type state struct {
	// LastHoldID is the id of the most recently created hold.
	LastHoldID string `json:"last_hold_id"`
}

// statePath returns the state file location under the user config directory.
func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vamoose", "state.json"), nil
}

// loadState reads the state file, returning a zero state when absent.
func loadState() (state, error) {
	path, err := statePath()
	if err != nil {
		return state{}, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state{}, nil
	}
	if err != nil {
		return state{}, err
	}
	var s state
	if err := json.Unmarshal(b, &s); err != nil {
		return state{}, err
	}
	return s, nil
}

// saveState writes the state file, creating parent directories as needed.
func saveState(s state) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// resolveHoldID returns the flag value, or the last created hold when empty.
func resolveHoldID(flagID string) (string, error) {
	if flagID != "" {
		return flagID, nil
	}
	s, err := loadState()
	if err != nil {
		return "", fmt.Errorf("load state: %w", err)
	}
	if s.LastHoldID == "" {
		return "", fmt.Errorf("no hold id: pass --id or run vamoose request first")
	}
	return s.LastHoldID, nil
}
