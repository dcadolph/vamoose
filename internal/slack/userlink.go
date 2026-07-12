package slack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dcadolph/vamoose/internal/secret"
	"github.com/dcadolph/vamoose/internal/util"
)

// ErrNotLinked reports that a Slack user has not linked a calendar.
var ErrNotLinked = errors.New("slack user not linked to a calendar")

// UserLink is a Slack user's linked calendar identity. The credential fields are
// sensitive: the backing store keeps them in a 0600 file and callers must never
// log them. RefreshToken serves the OAuth providers (google, graph); the ICloud
// fields serve icloud, which has no OAuth and uses an app-specific password.
type UserLink struct {
	// Provider is the calendar provider name: google, graph, or icloud.
	Provider string `json:"provider"`
	// RefreshToken is the OAuth refresh token for google and graph.
	RefreshToken string `json:"refresh_token,omitempty"`
	// ICloudUser is the Apple ID email for icloud.
	ICloudUser string `json:"icloud_user,omitempty"`
	// ICloudAppPassword is the app-specific password for icloud.
	ICloudAppPassword string `json:"icloud_app_password,omitempty"`
}

// Redacted returns a copy safe to log, with secret fields masked.
func (l UserLink) Redacted() UserLink {
	r := UserLink{Provider: l.Provider}
	if l.RefreshToken != "" {
		r.RefreshToken = "<redacted>"
	}
	if l.ICloudUser != "" {
		r.ICloudUser = l.ICloudUser
	}
	if l.ICloudAppPassword != "" {
		r.ICloudAppPassword = "<redacted>"
	}
	return r
}

// LinkID identifies a linked user by workspace and user id.
type LinkID struct {
	// Team is the Slack workspace id.
	Team string
	// User is the Slack user id.
	User string
}

// UserLinkStore persists a Slack user's linked calendar, keyed by workspace and
// user so each user in each workspace links their own calendar.
type UserLinkStore interface {
	// SaveLink records a user's linked calendar.
	SaveLink(teamID, userID string, link UserLink) error
	// GetLink returns a user's linked calendar, or ErrNotLinked when absent.
	GetLink(teamID, userID string) (UserLink, error)
	// DeleteLink removes a user's link, succeeding even when none exists.
	DeleteLink(teamID, userID string) error
	// List returns every linked user.
	List() ([]LinkID, error)
}

// linkKey builds the storage key for a workspace user.
func linkKey(teamID, userID string) string { return teamID + ":" + userID }

// UserLinkFileStore persists user links as a JSON map in a 0600 file, sealed with
// AES-256-GCM when an encryption box is set.
type UserLinkFileStore struct {
	// path is the file location.
	path string
	// box seals and opens the file when set, for a headless host; nil keeps plaintext.
	box *secret.Box
	// mu guards concurrent reads and writes.
	mu sync.Mutex
}

// NewUserLinkFileStore returns a plaintext user link store backed by the file at path.
func NewUserLinkFileStore(path string) *UserLinkFileStore {
	return &UserLinkFileStore{path: path}
}

// NewUserLinkStore returns a user link store that encrypts links at rest when
// VAMOOSE_SECRET_KEY is set, for a hosted server, otherwise a plaintext 0600 file.
func NewUserLinkStore(path string) (*UserLinkFileStore, error) {
	box, err := secret.FromEnv(os.Getenv)
	if errors.Is(err, secret.ErrNoKey) {
		return &UserLinkFileStore{path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	return &UserLinkFileStore{path: path, box: box}, nil
}

// SaveLink records a user's linked calendar, creating parent directories.
func (s *UserLinkFileStore) SaveLink(teamID, userID string, link UserLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	m[linkKey(teamID, userID)] = link
	return s.store(m)
}

// GetLink returns a user's linked calendar, or ErrNotLinked when absent.
func (s *UserLinkFileStore) GetLink(teamID, userID string) (UserLink, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return UserLink{}, err
	}
	link, ok := m[linkKey(teamID, userID)]
	if !ok {
		return UserLink{}, ErrNotLinked
	}
	return link, nil
}

// DeleteLink removes a user's link, succeeding even when none exists.
func (s *UserLinkFileStore) DeleteLink(teamID, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return err
	}
	delete(m, linkKey(teamID, userID))
	return s.store(m)
}

// List returns every linked user parsed from the store keys.
func (s *UserLinkFileStore) List() ([]LinkID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.load()
	if err != nil {
		return nil, err
	}
	ids := make([]LinkID, 0, len(m))
	for key := range m {
		if team, user, ok := strings.Cut(key, ":"); ok {
			ids = append(ids, LinkID{Team: team, User: user})
		}
	}
	return ids, nil
}

// load reads the link map, decrypting when a box is set, and returns an empty map when
// the file is absent.
func (s *UserLinkFileStore) load() (map[string]UserLink, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]UserLink{}, nil
	}
	if err != nil {
		return nil, err
	}
	if s.box != nil {
		b, err = s.box.Open(b)
		if err != nil {
			return nil, fmt.Errorf("decrypt user links: %w", err)
		}
	}
	m := map[string]UserLink{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse user links: %w", err)
	}
	return m, nil
}

// store writes the link map to the 0600 file, encrypting when a box is set.
func (s *UserLinkFileStore) store(m map[string]UserLink) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	var b []byte
	var err error
	if s.box != nil {
		b, err = json.Marshal(m)
	} else {
		b, err = json.MarshalIndent(m, "", "  ")
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
