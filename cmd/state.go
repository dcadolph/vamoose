package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dcadolph/vamoose/internal/util"
)

// state holds cross-invocation vamoose data, currently the last hold created.
type state struct {
	// LastHold references the most recently created hold and its provider.
	LastHold holdRef `json:"last_hold"`
}

// holdRef locates a hold by the provider that owns it and its provider event id.
type holdRef struct {
	// Provider is the registered calendar provider name that created the hold.
	Provider string `json:"provider"`
	// ID is the provider event identifier.
	ID string `json:"id"`
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
	return util.WriteFileAtomic(path, b, 0o600)
}

// resolveHold returns the hold to act on: an explicit id paired with the
// selected provider, or the last created hold and its provider when id is empty.
func resolveHold(flagID, flagProvider string) (holdRef, error) {
	if flagID != "" {
		return holdRef{Provider: resolveProvider(flagProvider), ID: flagID}, nil
	}
	s, err := loadState()
	if err != nil {
		return holdRef{}, fmt.Errorf("load state: %w", err)
	}
	if s.LastHold.ID == "" {
		return holdRef{}, fmt.Errorf("no hold id: pass --id or run vamoose request first")
	}
	return s.LastHold, nil
}
