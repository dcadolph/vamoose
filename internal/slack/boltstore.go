package slack

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dcadolph/vamoose/internal/boltstore"
)

// Buckets the Slack stores use within the shared database.
const (
	// tokenBucket holds per-workspace bot tokens keyed by team id.
	tokenBucket = "slack_tokens"
	// linkBucket holds per-user calendar links keyed by team:user.
	linkBucket = "slack_user_links"
)

// Interface checks: the bolt stores satisfy the same seams as the file stores.
var (
	_ TokenStore    = (*BoltTokenStore)(nil)
	_ UserLinkStore = (*BoltUserLinkStore)(nil)
)

// BoltTokenStore persists per-workspace bot tokens in a shared bbolt database, encrypted
// at rest when a key is set, for a hosted multi-tenant server.
type BoltTokenStore struct {
	// db is the shared database handle.
	db *boltstore.DB
}

// NewBoltTokenStore returns a bolt-backed token store over db.
func NewBoltTokenStore(db *boltstore.DB) *BoltTokenStore { return &BoltTokenStore{db: db} }

// Save records a workspace's bot token.
func (s *BoltTokenStore) Save(teamID, botToken string) error {
	return s.db.Put(tokenBucket, teamID, []byte(botToken))
}

// Get returns a workspace's bot token, or an error when it is not installed.
func (s *BoltTokenStore) Get(teamID string) (string, error) {
	v, ok, err := s.db.Get(tokenBucket, teamID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("workspace %s not installed", teamID)
	}
	return string(v), nil
}

// BoltUserLinkStore persists per-user calendar links in a shared bbolt database,
// encrypted at rest when a key is set.
type BoltUserLinkStore struct {
	// db is the shared database handle.
	db *boltstore.DB
}

// NewBoltUserLinkStore returns a bolt-backed user link store over db.
func NewBoltUserLinkStore(db *boltstore.DB) *BoltUserLinkStore { return &BoltUserLinkStore{db: db} }

// SaveLink records a user's linked calendar.
func (s *BoltUserLinkStore) SaveLink(teamID, userID string, link UserLink) error {
	b, err := json.Marshal(link)
	if err != nil {
		return err
	}
	return s.db.Put(linkBucket, linkKey(teamID, userID), b)
}

// GetLink returns a user's linked calendar, or ErrNotLinked when absent.
func (s *BoltUserLinkStore) GetLink(teamID, userID string) (UserLink, error) {
	v, ok, err := s.db.Get(linkBucket, linkKey(teamID, userID))
	if err != nil {
		return UserLink{}, err
	}
	if !ok {
		return UserLink{}, ErrNotLinked
	}
	var link UserLink
	if err := json.Unmarshal(v, &link); err != nil {
		return UserLink{}, err
	}
	return link, nil
}

// DeleteLink removes a user's link, succeeding even when none exists.
func (s *BoltUserLinkStore) DeleteLink(teamID, userID string) error {
	return s.db.Delete(linkBucket, linkKey(teamID, userID))
}

// List returns every linked user parsed from the store keys.
func (s *BoltUserLinkStore) List() ([]LinkID, error) {
	m, err := s.db.List(linkBucket)
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
