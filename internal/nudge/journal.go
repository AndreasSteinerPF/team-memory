// Package nudge owns the near-moment proposing/observing nudge engine (spec
// §2–4). It keeps a per-session journal under .git/tm/nudge, detects
// memory-worthy signals from hook events, and decides at most one nudge per
// turn. Local state only, never a ledger record — like internal/acks.
package nudge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// EditRecord is one Edit/Write to a path at a given turn.
type EditRecord struct {
	Path string `json:"path"`
	Turn int    `json:"turn"`
}

// CmdOutcome is one command invocation's signature and pass/fail at a turn.
type CmdOutcome struct {
	Signature string `json:"signature"` // normalized argv head (binary + subcommand)
	Failed    bool   `json:"failed"`
	Turn      int    `json:"turn"`
}

// Surfaced records a memory shown to this session for a path (by check-action).
type Surfaced struct {
	MemoryID string `json:"memory_id"`
	Path     string `json:"path"`
	Drift    bool   `json:"drift"` // surfaced with a drift annotation
}

// FiredNudge records a nudge already emitted, for dedup and budget.
type FiredNudge struct {
	Key  string `json:"key"` // "<signaltype>:<path-or-memory>"
	Turn int    `json:"turn"`
}

// Journal is the per-session local state. Keyed by session id, TTL-expired like
// acks. Never a ledger record.
type Journal struct {
	Session     string       `json:"session"`
	Turn        int          `json:"turn"`
	Edits       []EditRecord `json:"edits,omitempty"`
	Commands    []CmdOutcome `json:"commands,omitempty"`
	Reverts     []int        `json:"reverts,omitempty"`      // turns a revert happened
	Surfaced    []Surfaced   `json:"surfaced,omitempty"`     // memories shown this session
	PromptTurns []int        `json:"prompt_turns,omitempty"` // turns a user prompt landed
	Fired       []FiredNudge `json:"fired,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Store is a directory of journal files keyed by session id.
type Store struct{ dir string }

// Open creates (if needed) and returns the journal store under gitDir/tm/nudge.
func Open(gitDir string) (*Store, error) {
	dir := filepath.Join(gitDir, "tm", "nudge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(session string) string {
	return filepath.Join(s.dir, session+".json")
}

// Load returns the journal for session, or a fresh empty one if none exists.
func (s *Store) Load(session string) (*Journal, error) {
	data, err := os.ReadFile(s.path(session))
	if os.IsNotExist(err) {
		return &Journal{Session: session}, nil
	}
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return &Journal{Session: session}, nil // corrupt ⇒ start fresh
	}
	j.Session = session
	return &j, nil
}

// Save writes the journal atomically-enough for local single-writer use.
func (s *Store) Save(j *Journal) error {
	j.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(j.Session), data, 0o644)
}
