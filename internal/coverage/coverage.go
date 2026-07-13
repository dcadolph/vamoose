// Package coverage records time off booked through vamoose and answers who is off in a
// window, so a workflow can weigh team coverage before it books or approves more time off.
// It is backend-agnostic: it sees the time off vamoose manages, not manually entered
// calendar events, which is the set a team running its time off through vamoose cares
// about. The ledger persists as a JSON file, written atomically.
package coverage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/dcadolph/vamoose/internal/util"
)

// Entry is one person's booked time off.
type Entry struct {
	// Owner is the person off, by email.
	Owner string `json:"owner"`
	// HoldID is the calendar hold this entry came from, so a cancel can remove it.
	HoldID string `json:"hold_id,omitempty"`
	// Start is the first moment off.
	Start time.Time `json:"start"`
	// End is the exclusive end of the time off.
	End time.Time `json:"end"`
	// Subject is the hold's title, for display.
	Subject string `json:"subject,omitempty"`
}

// overlaps reports whether the entry's window intersects [start, end), both ends treated
// as half-open so touching windows do not count as overlapping.
func (e Entry) overlaps(start, end time.Time) bool {
	return e.Start.Before(end) && start.Before(e.End)
}

// Ledger records booked time off and answers coverage questions, persisted to a file.
type Ledger struct {
	// path is the JSON file location.
	path string
	// now returns the current time, injectable for tests.
	now func() time.Time
	// mu guards read-modify-write of the file.
	mu sync.Mutex
}

// NewLedger returns a ledger backed by the file at path.
func NewLedger(path string) *Ledger {
	return &Ledger{path: path, now: time.Now}
}

// Record adds an entry and drops any whose end is before now, so the file stays bounded
// to current and future time off. A zero-length or inverted window is rejected.
func (l *Ledger) Record(e Entry) error {
	if !e.End.After(e.Start) {
		return errors.New("coverage: end must be after start")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.load()
	if err != nil {
		return err
	}
	now := l.now()
	kept := make([]Entry, 0, len(entries)+1)
	for _, existing := range entries {
		if existing.End.After(now) {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, e)
	return l.store(kept)
}

// Remove drops the entry with the given hold id, if present. A cancel calls it so a
// withdrawn hold no longer counts against coverage. Removing an unknown id is not an error.
func (l *Ledger) Remove(holdID string) error {
	if holdID == "" {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.load()
	if err != nil {
		return err
	}
	kept := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.HoldID != holdID {
			kept = append(kept, e)
		}
	}
	if len(kept) == len(entries) {
		return nil
	}
	return l.store(kept)
}

// Overlapping returns the entries whose window intersects [start, end), excluding the
// given owner (pass an empty string to exclude no one), sorted by start.
func (l *Ledger) Overlapping(start, end time.Time, excludeOwner string) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entries, err := l.load()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if e.Owner == excludeOwner {
			continue
		}
		if e.overlaps(start, end) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// CountOff returns how many distinct people other than excludeOwner have time off
// overlapping [start, end).
func (l *Ledger) CountOff(start, end time.Time, excludeOwner string) (int, error) {
	entries, err := l.Overlapping(start, end, excludeOwner)
	if err != nil {
		return 0, err
	}
	owners := make(map[string]bool, len(entries))
	for _, e := range entries {
		owners[e.Owner] = true
	}
	return len(owners), nil
}

// load reads the ledger, returning an empty slice when the file is absent.
func (l *Ledger) load() ([]Entry, error) {
	b, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// store writes the ledger atomically, creating parent directories as needed.
func (l *Ledger) store(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return util.WriteFileAtomic(l.path, b, 0o600)
}
