// Package nudge owns the near-moment proposing/observing nudge engine
// (prd.md §10.1). It keeps a per-session journal under .git/tm/nudge, detects
// memory-worthy signals from hook events, and decides at most one nudge per
// turn. Local state only, never a ledger record — like internal/acks.
package nudge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

type DeliveryMode string

const (
	DeliveryRendered DeliveryMode = "rendered"
	DeliveryQueued   DeliveryMode = "queued"
)

// FiredNudge records a nudge already emitted, for dedup, budget, and reporting.
type FiredNudge struct {
	Key         string       `json:"key"` // "<signaltype>:<path-or-memory>"
	Turn        int          `json:"turn"`
	Type        SignalType   `json:"type,omitempty"`
	Verb        string       `json:"verb,omitempty"`
	Path        string       `json:"path,omitempty"`
	MemoryID    string       `json:"memory_id,omitempty"`
	TextBytes   int          `json:"text_bytes,omitempty"`
	Delivery    DeliveryMode `json:"delivery,omitempty"`
	FiredAt     time.Time    `json:"fired_at,omitempty"`
	DeliveredAt time.Time    `json:"delivered_at,omitempty"`
	// PendingDelivery marks a directly rendered attempt persisted before output.
	// It remains retryable until rendering succeeds and the delivered state saves.
	PendingDelivery bool `json:"pending_delivery,omitempty"`
	DrainedTurn     int  `json:"drained_turn,omitempty"`
}

// Journal is the per-session local state. Keyed by session id, TTL-expired like
// acks. Never a ledger record.
type Journal struct {
	Session      string        `json:"session"`
	Turn         int           `json:"turn"`
	Edits        []EditRecord  `json:"edits,omitempty"`
	Commands     []CmdOutcome  `json:"commands,omitempty"`
	Reverts      []int         `json:"reverts,omitempty"`      // turns a revert happened
	Surfaced     []Surfaced    `json:"surfaced,omitempty"`     // memories shown this session
	PromptTurns  []int         `json:"prompt_turns,omitempty"` // turns a user prompt landed
	Fired        []FiredNudge  `json:"fired,omitempty"`
	Suppressions []Suppression `json:"suppressions,omitempty"`
	Injected     []string      `json:"injected,omitempty"` // advisory memory ids delivered
	// Pending holds nudge text emitted at Stop that needs to be re-injected at
	// the next UserPromptSubmit. Stop-hook stdout does not actually surface to
	// the agent on Claude Code (contested 2026-06-17 — see ledger memory
	// 01KV84H0XQTPVWVNR65PG1TD2A), so on that harness we queue the rendered
	// text here and the prompt-signal hook drains it via additionalContext
	// (a channel verified to surface).
	Pending   []string  `json:"pending,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func FiredFromNudge(n Nudge, turn int, delivery DeliveryMode, firedAt time.Time) FiredNudge {
	return FiredNudge{
		Key:       n.Key,
		Turn:      turn,
		Type:      n.Type,
		Verb:      n.Verb,
		Path:      n.Path,
		MemoryID:  n.MemoryID,
		TextBytes: len([]byte(n.Text)),
		Delivery:  delivery,
		FiredAt:   firedAt,
	}
}

func (j *Journal) RecordSuppressions(s []Suppression) {
	j.Suppressions = append(j.Suppressions, s...)
}

func (j *Journal) MarkQueuedDrained(turn int, deliveredAt time.Time) {
	for i := range j.Fired {
		if j.Fired[i].Delivery == DeliveryQueued && j.Fired[i].DrainedTurn == 0 {
			j.Fired[i].DrainedTurn = turn
			j.Fired[i].DeliveredAt = deliveredAt
		}
	}
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

// RecordEdit logs an edit to path at the current turn.
func (j *Journal) RecordEdit(path string) {
	j.Edits = append(j.Edits, EditRecord{Path: path, Turn: j.Turn})
}

// EditCount returns how many edits to path this session.
func (j *Journal) EditCount(path string) int {
	n := 0
	for _, e := range j.Edits {
		if e.Path == path {
			n++
		}
	}
	return n
}

// RecordCommand logs a command outcome. A recognized revert/reset command also
// records a revert event at the current turn.
func (j *Journal) RecordCommand(command string, failed bool) {
	sig := Signature(command)
	j.Commands = append(j.Commands, CmdOutcome{Signature: sig, Failed: failed, Turn: j.Turn})
	if isRevert(command) {
		j.Reverts = append(j.Reverts, j.Turn)
	}
}

// AlreadyInjected reports whether memID's advisory was injected this session.
func (j *Journal) AlreadyInjected(memID string) bool {
	for _, id := range j.Injected {
		if id == memID {
			return true
		}
	}
	return false
}

// MarkInjected records that memID's advisory was injected (idempotent).
func (j *Journal) MarkInjected(memID string) {
	if !j.AlreadyInjected(memID) {
		j.Injected = append(j.Injected, memID)
	}
}

// RecordSurfaced logs that a memory was shown to this session for a path.
func (j *Journal) RecordSurfaced(memoryID, path string, drift bool) {
	for _, s := range j.Surfaced {
		if s.MemoryID == memoryID {
			return // already recorded
		}
	}
	j.Surfaced = append(j.Surfaced, Surfaced{MemoryID: memoryID, Path: path, Drift: drift})
}

// RecordPrompt logs that a user prompt landed at the current turn.
func (j *Journal) RecordPrompt() {
	j.PromptTurns = append(j.PromptTurns, j.Turn)
}

// Signature normalizes a command to its argv head (binary + subcommand), e.g.
// "go test ./..." → "go test", "pytest -q" → "pytest". Leading env assignments
// (FOO=bar) are skipped.
func Signature(command string) string {
	fields := strings.Fields(command)
	var head []string
	for _, f := range fields {
		if len(head) == 0 && strings.Contains(f, "=") && !strings.HasPrefix(f, "-") {
			continue // skip leading env assignment
		}
		if strings.HasPrefix(f, "-") {
			break
		}
		head = append(head, f)
		if len(head) == 2 {
			break
		}
	}
	return strings.Join(head, " ")
}

func isRevert(command string) bool {
	c := strings.ToLower(command)
	return strings.Contains(c, "git revert") ||
		strings.Contains(c, "git reset --hard") ||
		strings.Contains(c, "git checkout -- ") ||
		strings.Contains(c, "git restore")
}
