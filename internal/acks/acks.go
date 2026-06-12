// Package acks records local, session- or TTL-scoped acknowledgments of
// requirement memories under .git/tm/acks (prd.md §10.2). Acks are local state,
// never ledger records: they gate the hook, they are not evidence.
package acks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ack struct {
	MemoryID  string    `json:"memory_id"`
	Session   string    `json:"session,omitempty"`
	Note      string    `json:"note,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"` // zero ⇒ session-scoped
}

// Store is a directory of ack files keyed by memory id.
type Store struct{ dir string }

// Open creates (if needed) and returns the ack store under gitDir/tm/acks.
func Open(gitDir string) (*Store, error) {
	dir := filepath.Join(gitDir, "tm", "acks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(memID string) string {
	return filepath.Join(s.dir, memID+".json")
}

// Ack records acknowledgment of memID. If session is non-empty the ack is
// session-scoped; otherwise it expires ttl after now.
func (s *Store) Ack(memID, session, note string, ttl time.Duration, now time.Time) error {
	a := ack{MemoryID: memID, Session: session, Note: note}
	if session == "" {
		a.ExpiresAt = now.Add(ttl).UTC()
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(memID), data, 0o644)
}

// IsAcked reports whether memID has a live acknowledgment for session at now.
func (s *Store) IsAcked(memID, session string, now time.Time) (bool, error) {
	data, err := os.ReadFile(s.path(memID))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var a ack
	if err := json.Unmarshal(data, &a); err != nil {
		return false, nil
	}
	if a.Session != "" {
		return session != "" && session == a.Session, nil
	}
	return now.Before(a.ExpiresAt), nil
}
