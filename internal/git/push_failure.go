package git

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// PushFailureRecord is the on-disk shape of .git/tm/push_failure.json. It is
// local-only, never committed and never synced (spec §3.3, same convention as
// internal/acks and internal/nudge).
type PushFailureRecord struct {
	At            time.Time       `json:"at"`
	Remote        string          `json:"remote"`
	Kind          PushFailureKind `json:"kind"`
	StderrExcerpt string          `json:"stderr_excerpt"`
	Consecutive   int             `json:"consecutive"`
}

// PushFailureStore is a single-file store at <gitDir>/tm/push_failure.json.
type PushFailureStore struct{ path string }

// OpenPushFailureStore creates (if needed) the parent directory and returns a
// store handle. The file itself is not created until the first Record call.
func OpenPushFailureStore(gitDir string) (*PushFailureStore, error) {
	dir := filepath.Join(gitDir, "tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &PushFailureStore{path: filepath.Join(dir, "push_failure.json")}, nil
}

// stderrExcerptMaxBytes caps the recorded stderr to a reasonable preview.
// A multi-megabyte stderr blob serves nobody and could fill the user's .git.
const stderrExcerptMaxBytes = 4096

// Record writes a new failure record. If the previous record has the same
// (Remote, Kind), Consecutive bumps by one; otherwise it resets to one.
func (s *PushFailureStore) Record(remote string, kind PushFailureKind, stderr string, now time.Time) error {
	prev, _ := s.Read()
	consecutive := 1
	if prev != nil && prev.Remote == remote && prev.Kind == kind {
		consecutive = prev.Consecutive + 1
	}
	excerpt := stderr
	if len(excerpt) > stderrExcerptMaxBytes {
		excerpt = excerpt[:stderrExcerptMaxBytes]
	}
	rec := PushFailureRecord{
		At:            now.UTC(),
		Remote:        remote,
		Kind:          kind,
		StderrExcerpt: excerpt,
		Consecutive:   consecutive,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// Clear removes the record. No-op if the file is absent.
func (s *PushFailureStore) Clear() error {
	err := os.Remove(s.path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Read returns the current record, or nil if the file is absent or invalid.
// An invalid file is treated as absent (and is not removed; callers may decide).
func (s *PushFailureStore) Read() (*PushFailureRecord, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec PushFailureRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, nil
	}
	return &rec, nil
}

// ReadFresh returns the current record only if it is younger than maxAge. A
// stale record is pruned from disk and ReadFresh returns (nil, nil) — the
// caller does not need to follow up with a Clear.
func (s *PushFailureStore) ReadFresh(now time.Time, maxAge time.Duration) (*PushFailureRecord, error) {
	rec, err := s.Read()
	if err != nil || rec == nil {
		return rec, err
	}
	if now.Sub(rec.At) > maxAge {
		_ = s.Clear()
		return nil, nil
	}
	return rec, nil
}
