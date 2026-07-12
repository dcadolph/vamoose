package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dcadolph/vamoose/internal/secret"
	"github.com/dcadolph/vamoose/internal/util"
)

// defaultMax is how many events the file keeps before dropping the oldest, so the
// history stays bounded on a long-running daemon.
const defaultMax = 1000

// FileStore appends events to a JSON array file, sealed with AES-256-GCM when a box is
// set. It keeps at most max events, dropping the oldest.
type FileStore struct {
	// path is the history file location.
	path string
	// box seals and opens the file when set, for a headless host; nil keeps plaintext.
	box *secret.Box
	// max caps the retained events.
	max int
	// mu guards concurrent reads and writes within this process.
	mu sync.Mutex
}

// NewFileStore returns a history store that encrypts at rest when VAMOOSE_SECRET_KEY is
// set, for a hosted server, otherwise a plaintext 0600 file.
func NewFileStore(path string) (*FileStore, error) {
	s := &FileStore{path: path, max: defaultMax}
	box, err := secret.FromEnv(os.Getenv)
	if errors.Is(err, secret.ErrNoKey) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	s.box = box
	return s, nil
}

// Record appends an event, capping the retained history and creating parent directories.
func (s *FileStore) Record(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.load()
	if err != nil {
		return err
	}
	events = append(events, e)
	if s.max > 0 && len(events) > s.max {
		events = events[len(events)-s.max:]
	}
	return s.store(events)
}

// Events returns the recorded history in order, newest last.
func (s *FileStore) Events() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// load reads the history, decrypting when a box is set, and returns nil when the file is
// absent.
func (s *FileStore) load() ([]Event, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if s.box != nil {
		b, err = s.box.Open(b)
		if err != nil {
			return nil, fmt.Errorf("decrypt audit log: %w", err)
		}
	}
	var events []Event
	if err := json.Unmarshal(b, &events); err != nil {
		return nil, fmt.Errorf("parse audit log: %w", err)
	}
	return events, nil
}

// store writes the history to the 0600 file, encrypting when a box is set.
func (s *FileStore) store(events []Event) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	var b []byte
	var err error
	if s.box != nil {
		b, err = json.Marshal(events)
	} else {
		b, err = json.MarshalIndent(events, "", "  ")
	}
	if err != nil {
		return err
	}
	if s.box != nil {
		if b, err = s.box.Seal(b); err != nil {
			return err
		}
	}
	return util.WriteFileAtomic(s.path, b, 0o600)
}
