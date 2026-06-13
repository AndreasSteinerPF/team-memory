# Nudge Engine (Claude Code) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a near-moment nudge engine that escalates memory-worthy moments to pointed `tm_propose`/`tm_observe` prompts, delivered deterministically on Claude Code via PostToolUse (signal recording) and Stop (nudge emission) hooks, while keeping the verbs voluntary.

**Architecture:** A new `internal/nudge` package owns a per-session journal under `.git/tm/nudge/<session>.json` (modeled on `internal/acks`), pure signal-detection functions, and a decision/anti-spam policy. Two new CLI hook verbs wire it to Claude Code: `tm signal --hook` (PostToolUse) records signals; `tm nudge --hook` (Stop) emits at most one nudge per turn. The existing `tm check-action --hook` is extended to record surfaced memories into the journal so the observe signals have a source. The nudge package has **no** ledger/index dependency — the "did the agent already act?" check is injected as a predicate, keeping detection unit-testable.

**Tech Stack:** Go, cobra (CLI), `encoding/json` (journal + hook I/O), `gopkg.in/yaml.v3` (policy config), testscript/txtar (e2e). Follows existing `internal/acks`, `internal/cli/checkaction.go`, and `internal/policy` patterns.

This plan is slice 1 of 3 (see spec §6 build order). It delivers the complete nudge feature on Claude Code. Cross-harness adapters (Codex, Copilot, Cursor, Gemini) and PostToolUse advisory injection are Plans 2–3.

**Spec:** `docs/superpowers/specs/2026-06-14-cross-harness-memory-engine-design.md` (§2 architecture, §3 signals, §4 policy, §6.1 Claude Code, §7 config, §8 testing).

---

## File structure

- **Create** `internal/nudge/journal.go` — `Journal`, `Store`, `Load`/`Save`, recording mutators. The local per-session state.
- **Create** `internal/nudge/journal_test.go` — round-trip + mutator tests.
- **Create** `internal/nudge/signal.go` — `Signal`, `SignalType`, pure `Detect(j, cfg)`.
- **Create** `internal/nudge/signal_test.go` — table-driven per-signal detection.
- **Create** `internal/nudge/policy.go` — `Decide(j, cfg, acted)` + nudge text rendering.
- **Create** `internal/nudge/policy_test.go` — suppress/dedup/cooldown/priority/cadence.
- **Modify** `internal/policy/policy.go` — add `Nudge` config struct + defaults.
- **Modify** `internal/policy/policy_test.go` — default-value assertions.
- **Create** `internal/cli/signal.go` — `tm signal --hook` (PostToolUse recorder).
- **Create** `internal/cli/nudge.go` — `tm nudge --hook` (Stop emitter).
- **Modify** `internal/cli/checkaction.go` — record surfaced memories into the journal in hook mode.
- **Modify** `internal/cli/cli.go:34-53` — register the two new commands.
- **Modify** `internal/cli/env.go` — add `nudgeStore()` helper.
- **Create** `e2e/testdata/scripts/nudge.txtar` — end-to-end lifecycle.
- **Modify** `prd.md` — §10, §149, §537, config section.

---

## Task 1: Nudge config in policy

**Files:**
- Modify: `internal/policy/policy.go:11-18` (add field), `:55-57` area (add struct), `:60-90` (defaults)
- Test: `internal/policy/policy_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/policy/policy_test.go`:

```go
func TestDefaultNudgeConfig(t *testing.T) {
	p := policy.Default()
	if !p.Nudge.Enabled {
		t.Errorf("Nudge.Enabled default = false, want true")
	}
	if p.Nudge.MaxPerSession != 3 {
		t.Errorf("Nudge.MaxPerSession = %d, want 3", p.Nudge.MaxPerSession)
	}
	if p.Nudge.CooldownTurns != 3 {
		t.Errorf("Nudge.CooldownTurns = %d, want 3", p.Nudge.CooldownTurns)
	}
	if p.Nudge.SelfReviewEvery != 8 {
		t.Errorf("Nudge.SelfReviewEvery = %d, want 8", p.Nudge.SelfReviewEvery)
	}
	if p.Nudge.ChurnThreshold != 3 {
		t.Errorf("Nudge.ChurnThreshold = %d, want 3", p.Nudge.ChurnThreshold)
	}
}

func TestNudgeConfigRoundTripsThroughYAML(t *testing.T) {
	data, err := policy.DefaultYAML()
	if err != nil {
		t.Fatal(err)
	}
	p, err := policy.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nudge != policy.Default().Nudge {
		t.Errorf("Nudge round-trip = %+v, want %+v", p.Nudge, policy.Default().Nudge)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/policy/ -run TestDefaultNudgeConfig -v`
Expected: FAIL — `p.Nudge undefined (type policy.Policy has no field or method Nudge)`.

- [ ] **Step 3: Add the config struct and field**

In `internal/policy/policy.go`, add `Nudge` to the `Policy` struct (after `Sync`):

```go
	Sync                   Sync                            `yaml:"sync"`
	Nudge                  Nudge                           `yaml:"nudge"`
```

Add the struct after the `Sync` type definition:

```go
// Nudge configures the near-moment proposing/observing nudge engine (spec §4, §7).
type Nudge struct {
	Enabled         bool `yaml:"enabled"`
	MaxPerSession   int  `yaml:"max_per_session"`
	CooldownTurns   int  `yaml:"cooldown_turns"`
	SelfReviewEvery int  `yaml:"self_review_every"`
	ChurnThreshold  int  `yaml:"churn_threshold"`
}
```

Add to the `Default()` return literal (after `Sync: ...`):

```go
		Nudge: Nudge{
			Enabled:         true,
			MaxPerSession:   3,
			CooldownTurns:   3,
			SelfReviewEvery: 8,
			ChurnThreshold:  3,
		},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/ -v`
Expected: PASS (all, including the two new tests).

- [ ] **Step 5: Commit**

```bash
git add internal/policy/policy.go internal/policy/policy_test.go
git commit -m "feat(nudge): add nudge engine config to policy (spec §7)"
```

---

## Task 2: Journal types, Store, Load/Save

**Files:**
- Create: `internal/nudge/journal.go`
- Test: `internal/nudge/journal_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nudge/journal_test.go`:

```go
package nudge_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func TestStoreLoadMissingReturnsEmptyJournal(t *testing.T) {
	s, err := nudge.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Load("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if j.Session != "sess-1" {
		t.Errorf("Session = %q, want sess-1", j.Session)
	}
	if len(j.Edits) != 0 || j.Turn != 0 {
		t.Errorf("fresh journal not empty: %+v", j)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	s, err := nudge.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, _ := s.Load("sess-1")
	j.Turn = 4
	j.Edits = append(j.Edits, nudge.EditRecord{Path: "a/b.go", Turn: 2})
	if err := s.Save(j); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Turn != 4 || len(got.Edits) != 1 || got.Edits[0].Path != "a/b.go" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nudge/ -run TestStore -v`
Expected: FAIL — `package .../internal/nudge is not in std` / no Go files (the package does not exist yet).

- [ ] **Step 3: Write the journal implementation**

Create `internal/nudge/journal.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nudge/ -run TestStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nudge/journal.go internal/nudge/journal_test.go
git commit -m "feat(nudge): per-session journal store under .git/tm/nudge (spec §2)"
```

---

## Task 3: Journal recording mutators

**Files:**
- Modify: `internal/nudge/journal.go`
- Test: `internal/nudge/journal_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/nudge/journal_test.go`:

```go
func TestRecordEditCounts(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordEdit("x.go")
	j.RecordEdit("x.go")
	if n := j.EditCount("x.go"); n != 2 {
		t.Errorf("EditCount = %d, want 2", n)
	}
}

func TestRecordCommandSignature(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordCommand("go test ./...", true)
	if len(j.Commands) != 1 || j.Commands[0].Signature != "go test" || !j.Commands[0].Failed {
		t.Errorf("command not recorded: %+v", j.Commands)
	}
}

func TestRecordCommandDetectsRevert(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 5}
	j.RecordCommand("git reset --hard HEAD~1", false)
	if len(j.Reverts) != 1 || j.Reverts[0] != 5 {
		t.Errorf("revert not recorded: %+v", j.Reverts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nudge/ -run TestRecord -v`
Expected: FAIL — `j.RecordEdit undefined`.

- [ ] **Step 3: Add the mutators**

Append to `internal/nudge/journal.go`:

```go
import "strings" // add to the existing import block

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
```

Note: merge the `"strings"` import into the existing import block rather than adding a second block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nudge/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nudge/journal.go internal/nudge/journal_test.go
git commit -m "feat(nudge): journal recording mutators + command signature (spec §3)"
```

---

## Task 4: Signal detection

**Files:**
- Create: `internal/nudge/signal.go`
- Test: `internal/nudge/signal_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nudge/signal_test.go`:

```go
package nudge_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func has(sigs []nudge.Signal, t nudge.SignalType) *nudge.Signal {
	for i := range sigs {
		if sigs[i].Type == t {
			return &sigs[i]
		}
	}
	return nil
}

func TestDetectFailPassRequiresEditBetween(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	// fail at turn 1, edit, pass at turn 2 → signal
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test ./...", true)
	j.Turn = 2
	j.RecordEdit("internal/index/index.go")
	j.RecordCommand("go test ./...", false)
	if has(nudge.Detect(j, cfg), nudge.SigFailPass) == nil {
		t.Error("expected fail_pass signal")
	}

	// fail then pass with NO edit between → no signal (transient)
	j2 := &nudge.Journal{Session: "s"}
	j2.Turn = 1
	j2.RecordCommand("go test ./...", true)
	j2.RecordCommand("go test ./...", false)
	if has(nudge.Detect(j2, cfg), nudge.SigFailPass) != nil {
		t.Error("did not expect fail_pass without an edit between")
	}
}

func TestDetectChurn(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s"}
	for i := 0; i < 3; i++ {
		j.Turn = i
		j.RecordEdit("hot.go")
	}
	if has(nudge.Detect(j, cfg), nudge.SigChurn) == nil {
		t.Error("expected churn signal at threshold 3")
	}
}

func TestDetectRevert(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordCommand("git reset --hard HEAD~1", false)
	if has(nudge.Detect(j, cfg), nudge.SigRevert) == nil {
		t.Error("expected revert signal")
	}
}

func TestDetectSurfacedButUnobserved(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM1", "billing/migrations/x.sql", false)
	j.RecordEdit("billing/migrations/x.sql")
	s := has(nudge.Detect(j, cfg), nudge.SigUnobserved)
	if s == nil || s.Memory != "MEM1" {
		t.Errorf("expected unobserved signal for MEM1, got %+v", s)
	}
}

func TestDetectDriftAnchorEdited(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM2", "core/api.go", true) // drift=true
	j.RecordEdit("core/api.go")
	if has(nudge.Detect(j, cfg), nudge.SigDrift) == nil {
		t.Error("expected drift signal when a drifted anchor is edited")
	}
}

func TestDetectUserIntervened(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordEdit("auth/login.go")
	j.Turn = 2
	j.RecordPrompt()
	j.Turn = 3
	j.RecordEdit("auth/login.go")
	s := has(nudge.Detect(j, cfg), nudge.SigIntervened)
	if s == nil || s.Path != "auth/login.go" {
		t.Errorf("expected intervened signal for auth/login.go, got %+v", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nudge/ -run TestDetect -v`
Expected: FAIL — `nudge.Config undefined` / `nudge.Detect undefined`.

- [ ] **Step 3: Write the signal detection**

Create `internal/nudge/signal.go`:

```go
package nudge

// SignalType identifies a memory-worthy moment.
type SignalType string

const (
	SigFailPass   SignalType = "fail_pass"   // Tier A
	SigRevert     SignalType = "revert"      // Tier A
	SigChurn      SignalType = "churn"       // Tier A
	SigUnobserved SignalType = "unobserved"  // Tier A (observe)
	SigDrift      SignalType = "drift"       // Tier A (observe)
	SigIntervened SignalType = "intervened"  // Tier B (attention-flag)
)

// Config is the subset of policy.Nudge the detector and decider need. The CLI
// maps policy.Nudge → Config so the nudge package stays free of policy imports.
type Config struct {
	Enabled         bool
	MaxPerSession   int
	CooldownTurns   int
	SelfReviewEvery int
	ChurnThreshold  int
}

// Signal is one detected moment. Verb/MemType drive the nudge wording; Path and
// Memory key dedup and the suppress-if-acted check.
type Signal struct {
	Type    SignalType
	Verb    string // "propose" | "observe"
	MemType string // suggested memory type for propose signals
	Path    string // primary path (for dedup + acted check)
	Memory  string // memory id (for observe signals)
	Command string // command signature (for fail_pass wording)
}

// Key is the dedup key for a signal: one nudge per (type, path-or-memory).
func (s Signal) Key() string {
	id := s.Path
	if s.Memory != "" {
		id = s.Memory
	}
	return string(s.Type) + ":" + id
}

// Detect returns all signals currently present in the journal. Pure: no I/O,
// no ledger access. Suppress-if-acted and budget are applied later in Decide.
func Detect(j *Journal, cfg Config) []Signal {
	var out []Signal
	out = append(out, detectFailPass(j)...)
	out = append(out, detectRevert(j)...)
	out = append(out, detectChurn(j, cfg)...)
	out = append(out, detectObserve(j)...)
	out = append(out, detectIntervened(j)...)
	return out
}

// detectFailPass: a command signature that failed, then succeeded, with at
// least one edit between the two runs. Only the boolean transition matters.
func detectFailPass(j *Journal) []Signal {
	var out []Signal
	for i, fail := range j.Commands {
		if !fail.Failed {
			continue
		}
		for _, pass := range j.Commands[i+1:] {
			if pass.Failed || pass.Signature != fail.Signature {
				continue
			}
			if !editBetween(j, fail.Turn, pass.Turn) {
				continue
			}
			out = append(out, Signal{
				Type: SigFailPass, Verb: "propose", MemType: "failed_attempt",
				Path: lastEditBetween(j, fail.Turn, pass.Turn), Command: fail.Signature,
			})
			break
		}
	}
	return out
}

func editBetween(j *Journal, lo, hi int) bool {
	for _, e := range j.Edits {
		if e.Turn >= lo && e.Turn <= hi {
			return true
		}
	}
	return false
}

func lastEditBetween(j *Journal, lo, hi int) string {
	path := ""
	for _, e := range j.Edits {
		if e.Turn >= lo && e.Turn <= hi {
			path = e.Path
		}
	}
	return path
}

func detectRevert(j *Journal) []Signal {
	var out []Signal
	for range j.Reverts {
		out = append(out, Signal{Type: SigRevert, Verb: "propose", MemType: "failed_attempt"})
		break // one revert signal per session is enough; dedup handles the rest
	}
	return out
}

func detectChurn(j *Journal, cfg Config) []Signal {
	counts := map[string]int{}
	for _, e := range j.Edits {
		counts[e.Path]++
	}
	var out []Signal
	for path, n := range counts {
		if n >= cfg.ChurnThreshold {
			out = append(out, Signal{Type: SigChurn, Verb: "propose", MemType: "fragile_area", Path: path})
		}
	}
	return out
}

// detectObserve emits unobserved/drift signals for surfaced memories whose path
// the session subsequently edited.
func detectObserve(j *Journal) []Signal {
	var out []Signal
	for _, s := range j.Surfaced {
		if j.EditCount(s.Path) == 0 {
			continue
		}
		if s.Drift {
			out = append(out, Signal{Type: SigDrift, Verb: "observe", Path: s.Path, Memory: s.MemoryID})
		} else {
			out = append(out, Signal{Type: SigUnobserved, Verb: "observe", Path: s.Path, Memory: s.MemoryID})
		}
	}
	return out
}

// detectIntervened: edit P → user prompt → edit P again (Tier B). The lesson
// content lives in the user's words, so this only aims the self-review.
func detectIntervened(j *Journal) []Signal {
	var out []Signal
	for path := range pathsEditedAround(j) {
		out = append(out, Signal{Type: SigIntervened, Verb: "", Path: path})
	}
	return out
}

func pathsEditedAround(j *Journal) map[string]struct{} {
	found := map[string]struct{}{}
	for _, p := range j.PromptTurns {
		var before, after bool
		var path string
		for _, e := range j.Edits {
			if e.Turn < p {
				path, before = e.Path, true
			}
		}
		for _, e := range j.Edits {
			if e.Turn > p && e.Path == path {
				after = true
			}
		}
		if before && after && path != "" {
			found[path] = struct{}{}
		}
	}
	return found
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nudge/ -run TestDetect -v`
Expected: PASS (all six detect tests).

- [ ] **Step 5: Commit**

```bash
git add internal/nudge/signal.go internal/nudge/signal_test.go
git commit -m "feat(nudge): pure signal detection for all six signals (spec §3)"
```

---

## Task 5: Decision & anti-spam policy

**Files:**
- Create: `internal/nudge/policy.go`
- Test: `internal/nudge/policy_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/nudge/policy_test.go`:

```go
package nudge_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func cfg() nudge.Config {
	return nudge.Config{Enabled: true, MaxPerSession: 3, CooldownTurns: 3, SelfReviewEvery: 8, ChurnThreshold: 3}
}

func never(nudge.Signal) bool { return false }

func TestDecideEmitsPointedNudgeForFailPass(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("internal/index/x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok {
		t.Fatal("expected a nudge")
	}
	if !strings.Contains(out.Text, "tm_propose") || !strings.Contains(out.Text, "failed_attempt") {
		t.Errorf("nudge text missing propose guidance: %q", out.Text)
	}
}

func TestDecideSuppressesWhenActed(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3
	always := func(nudge.Signal) bool { return true }
	if _, ok := nudge.Decide(j, cfg(), always); ok {
		t.Error("expected suppression when the agent already acted")
	}
}

func TestDecideObserveOutranksPropose(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM1", "a.go", false) // → observe
	j.RecordEdit("a.go")
	for i := 0; i < 3; i++ { // also churn on b.go → propose
		j.Turn = i
		j.RecordEdit("b.go")
	}
	j.Turn = 5
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok || out.Verb != "observe" {
		t.Errorf("expected observe to win, got %+v ok=%v", out, ok)
	}
}

func TestDecideRespectsCooldown(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 2}
	j.Fired = append(j.Fired, nudge.FiredNudge{Key: "prior", Turn: 1}) // 1 turn ago < cooldown 3
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}
	if _, ok := nudge.Decide(j, cfg(), never); ok {
		t.Error("expected cooldown to suppress a nudge")
	}
}

func TestDecideRespectsMaxPerSession(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 20}
	j.Fired = []nudge.FiredNudge{{Turn: 1}, {Turn: 5}, {Turn: 9}} // already 3
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}
	if _, ok := nudge.Decide(j, cfg(), never); ok {
		t.Error("expected max-per-session ceiling to suppress")
	}
}

func TestDecidePeriodicSelfReview(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 9} // 9 >= SelfReviewEvery, no prior nudge
	j.RecordEdit("a.go")                       // session has ≥1 edit
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok || !strings.Contains(out.Text, "memory-worthy") {
		t.Errorf("expected a periodic self-review, got %+v ok=%v", out, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/nudge/ -run TestDecide -v`
Expected: FAIL — `nudge.Decide undefined`.

- [ ] **Step 3: Write the decision policy**

Create `internal/nudge/policy.go`:

```go
package nudge

import "fmt"

// Nudge is the engine's output: one line of context to inject, plus metadata so
// the caller can record it in the journal for dedup/budget.
type Nudge struct {
	Text string
	Verb string // "propose" | "observe" | "" (self-review)
	Key  string // dedup key recorded into FiredNudge
}

// priority orders surviving signals: observe signals first (they unblock
// provisional→active and decay if neglected), then fail_pass > revert > churn.
var priority = map[SignalType]int{
	SigUnobserved: 0, SigDrift: 1, SigFailPass: 2, SigRevert: 3, SigChurn: 4,
}

// Decide applies the anti-spam policy and returns at most one nudge for the
// turn. acted(s) reports whether the agent already proposed/observed for signal
// s this session (injected so this package needs no ledger/index dependency).
func Decide(j *Journal, cfg Config, acted func(Signal) bool) (Nudge, bool) {
	if !cfg.Enabled {
		return Nudge{}, false
	}
	if len(j.Fired) >= cfg.MaxPerSession {
		return Nudge{}, false
	}
	if lastFiredTurn(j) >= 0 && j.Turn-lastFiredTurn(j) < cfg.CooldownTurns {
		return Nudge{}, false
	}

	sigs := Detect(j, cfg)

	// Tier A: self-classifying. Drop already-nudged (dedup) and already-acted.
	best, ok := bestTierA(j, sigs, acted)
	if ok {
		return renderTierA(best), true
	}

	// Tier B: attention-flag → aimed self-review.
	for _, s := range sigs {
		if s.Type == SigIntervened && !firedKey(j, s.Key()) {
			return Nudge{
				Text: fmt.Sprintf("tm: the user redirected you while editing %s — was there a constraint or decision worth recording? If so, tm_propose it; otherwise ignore.", s.Path),
				Verb: "", Key: s.Key(),
			}, true
		}
	}

	// Periodic generic self-review.
	if j.Turn >= cfg.SelfReviewEvery && len(j.Edits) > 0 {
		return Nudge{
			Text: "tm: anything memory-worthy this session — a non-obvious failure, a hidden constraint, a fragile area? If so, tm_propose it; otherwise ignore.",
			Verb: "", Key: fmt.Sprintf("self_review:%d", j.Turn),
		}, true
	}
	return Nudge{}, false
}

func bestTierA(j *Journal, sigs []Signal, acted func(Signal) bool) (Signal, bool) {
	var best Signal
	found := false
	for _, s := range sigs {
		if s.Type == SigIntervened {
			continue
		}
		if firedKey(j, s.Key()) || acted(s) {
			continue
		}
		if !found || priority[s.Type] < priority[best.Type] {
			best, found = s, true
		}
	}
	return best, found
}

func renderTierA(s Signal) Nudge {
	var text string
	switch s.Type {
	case SigFailPass:
		text = fmt.Sprintf("tm: recovered from a failing `%s` after edits in %s — if that fix encodes a non-obvious lesson, tm_propose a failed_attempt; otherwise ignore.", s.Command, s.Path)
	case SigRevert:
		text = "tm: you reverted work this session — if an approach failed in a non-obvious way, tm_propose a failed_attempt; otherwise ignore."
	case SigChurn:
		text = fmt.Sprintf("tm: %s fought back (edited repeatedly) — if it's a fragile area, tm_propose a fragile_area; otherwise ignore.", s.Path)
	case SigUnobserved:
		text = fmt.Sprintf("tm: you were shown memory %s for %s and your work bears on it — tm_observe to confirm or contradict it (with evidence); otherwise ignore.", s.Memory, s.Path)
	case SigDrift:
		text = fmt.Sprintf("tm: memory %s is anchored to %s, which has drifted, and you just edited it — tm_observe mark_stale or adjust_scope; otherwise ignore.", s.Memory, s.Path)
	}
	return Nudge{Text: text, Verb: s.Verb, Key: s.Key()}
}

func lastFiredTurn(j *Journal) int {
	last := -1
	for _, f := range j.Fired {
		if f.Turn > last {
			last = f.Turn
		}
	}
	return last
}

func firedKey(j *Journal, key string) bool {
	for _, f := range j.Fired {
		if f.Key == key {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/nudge/ -v`
Expected: PASS (all journal, signal, and policy tests).

- [ ] **Step 5: Commit**

```bash
git add internal/nudge/policy.go internal/nudge/policy_test.go
git commit -m "feat(nudge): decision + anti-spam policy with priority and budget (spec §4)"
```

---

## Task 6: `nudgeStore` env helper + config mapping

**Files:**
- Modify: `internal/cli/env.go:84-87` (after `ackStore`)
- Test: covered by Task 7/9 (helper is exercised through the hook commands)

- [ ] **Step 1: Add the helper and config mapping**

In `internal/cli/env.go`, add the import:

```go
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
```

Add after the `ackStore` method:

```go
// nudgeStore opens the local nudge journal store under .git/tm/nudge.
func (e *env) nudgeStore() (*nudge.Store, error) {
	return nudge.Open(e.gitDir)
}

// nudgeConfig maps the policy's nudge settings onto the nudge package's Config.
func (e *env) nudgeConfig() nudge.Config {
	n := e.pol.Nudge
	return nudge.Config{
		Enabled:         n.Enabled,
		MaxPerSession:   n.MaxPerSession,
		CooldownTurns:   n.CooldownTurns,
		SelfReviewEvery: n.SelfReviewEvery,
		ChurnThreshold:  n.ChurnThreshold,
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: builds clean (helper unused until Task 7 — Go allows unused methods).

- [ ] **Step 3: Commit**

```bash
git add internal/cli/env.go
git commit -m "feat(nudge): env helpers for journal store and config mapping"
```

---

## Task 7: `tm signal --hook` (PostToolUse recorder)

**Files:**
- Create: `internal/cli/signal.go`
- Modify: `internal/cli/cli.go:34-53` (register)
- Test: `e2e/testdata/scripts/nudge.txtar` (Task 9) + an inline unit below

- [ ] **Step 1: Write the failing test**

Create `internal/cli/signal_test.go`:

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// runInit sets up a ledger in a fresh repo dir; returns the repo dir.
func runSignal(t *testing.T, repo, stdin string) int {
	t.Helper()
	var out, errb bytes.Buffer
	return cli.Run([]string{"--repo", repo, "signal", "--hook"}, strings.NewReader(stdin), &out, &errb)
}

func TestSignalHookRecordsCommandOutcome(t *testing.T) {
	repo := initRepo(t) // helper defined in cli's existing test support; see note
	// A failing test command followed (next call) by an edit + passing command.
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
	// The journal now holds two command outcomes and one edit for session s1.
	// Asserted indirectly via the nudge hook in Task 9's e2e; here we assert the
	// command exits 0 and emits nothing (recorder is silent).
}
```

First create the shared helper file `internal/cli/testhelpers_test.go` (skip if an equivalent already exists in the `cli_test` package — check `doctor_test.go`/`plugin_test.go` first):

```go
package cli_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// initRepo creates a temp git repo and runs `tm init`, returning the repo dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", dir, "init"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("tm init failed (%d): %s", code, errb.String())
	}
	return dir
}

// runSignalForTest feeds a PostToolUse event to `tm signal --hook`, asserting success.
func runSignalForTest(t *testing.T, repo, stdin string) {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "signal", "--hook"}, strings.NewReader(stdin), &out, &errb); code != 0 {
		t.Fatalf("signal hook failed (%d): %s", code, errb.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestSignalHook -v`
Expected: FAIL — unknown command `"signal"` (exit 1).

- [ ] **Step 3: Write the signal command**

Create `internal/cli/signal.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// postHookInput is the PostToolUse event contract (Claude Code). tool_response
// carries the exit code for Bash; nil exit_code ⇒ treat as success.
type postHookInput struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath string `json:"file_path"`
		Command  string `json:"command"`
	} `json:"tool_input"`
	ToolResponse struct {
		ExitCode *int `json:"exit_code"`
	} `json:"tool_response"`
}

func newSignalCmd(g *globalOpts) *cobra.Command {
	var hook bool
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Record nudge signals from a PostToolUse event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("signal currently supports only --hook mode")
			}
			var in postHookInput
			if err := json.NewDecoder(cmd.InOrStdin()).Decode(&in); err != nil {
				return fmt.Errorf("signal hook: decode stdin: %w", err)
			}
			if in.SessionID == "" {
				return nil // cannot key a journal without a session
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			store, err := e.nudgeStore()
			if err != nil {
				return err
			}
			j, err := store.Load(in.SessionID)
			if err != nil {
				return err
			}
			j.Turn++ // each PostToolUse advances the within-session clock

			switch {
			case in.ToolInput.Command != "":
				failed := in.ToolResponse.ExitCode != nil && *in.ToolResponse.ExitCode != 0
				j.RecordCommand(in.ToolInput.Command, failed)
			case in.ToolInput.FilePath != "":
				j.RecordEdit(relPath(e, in.ToolInput.FilePath))
			}
			return store.Save(j)
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a PostToolUse event on stdin and record signals")
	return cmd
}

// relPath converts an absolute or repo-relative path to a forward-slash repo path.
func relPath(e *env, p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		if r, err := filepath.Rel(e.repoDir, abs); err == nil {
			return filepath.ToSlash(r)
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}
```

Register in `internal/cli/cli.go` (inside the `root.AddCommand(...)` list):

```go
		newSignalCmd(g),
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestSignalHook -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/signal.go internal/cli/signal_test.go internal/cli/cli.go
git commit -m "feat(nudge): tm signal --hook records PostToolUse signals (spec §6.1)"
```

---

## Task 8: `tm nudge --hook` (Stop emitter) + record surfaced memories

**Files:**
- Create: `internal/cli/nudge.go`
- Modify: `internal/cli/cli.go` (register), `internal/cli/checkaction.go:200-206` (record surfaced)
- Test: `internal/cli/nudge_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/nudge_test.go`:

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func runNudge(t *testing.T, repo, stdin string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "--hook"}, strings.NewReader(stdin), &out, &errb)
	return out.String(), code
}

func TestNudgeHookEmitsAfterFailPass(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) } // see note
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	out, code := runNudge(t, repo, `{"session_id":"s1"}`)
	if code != 0 {
		t.Fatalf("nudge hook exit = %d", code)
	}
	if !strings.Contains(out, "tm_propose") || !strings.Contains(out, "failed_attempt") {
		t.Errorf("expected a propose nudge, got: %q", out)
	}
}

func TestNudgeHookSilentWithNoSignal(t *testing.T) {
	repo := initRepo(t)
	out, code := runNudge(t, repo, `{"session_id":"fresh"}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected silence on a fresh session, got: %q", out)
	}
}
```

> **Note:** `runSignalForTest` and `initRepo` are already defined in `internal/cli/testhelpers_test.go` (Task 7). Reuse them — do not redefine.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestNudgeHook -v`
Expected: FAIL — unknown command `"nudge"`.

- [ ] **Step 3: Write the nudge command**

Create `internal/cli/nudge.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

type stopHookInput struct {
	SessionID string `json:"session_id"`
}

// stopHookOutput injects the nudge as additional context at turn end.
//
// VERIFY (spec §10): confirm the Stop-hook context-injection shape on the
// installed Claude Code version against a live payload. Some versions surface
// Stop stdout directly; others require {"decision":"block","reason":...} (which
// forces a turn — undesirable for a low-pressure nudge). This output struct
// isolates that decision to one place; adjust here if the live payload differs.
type stopHookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func newNudgeCmd(g *globalOpts) *cobra.Command {
	var hook bool
	cmd := &cobra.Command{
		Use:   "nudge",
		Short: "Emit a proposing/observing nudge from a Stop event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("nudge currently supports only --hook mode")
			}
			var in stopHookInput
			if err := json.NewDecoder(cmd.InOrStdin()).Decode(&in); err != nil {
				return fmt.Errorf("nudge hook: decode stdin: %w", err)
			}
			if in.SessionID == "" {
				return nil
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			store, err := e.nudgeStore()
			if err != nil {
				return err
			}
			j, err := store.Load(in.SessionID)
			if err != nil {
				return err
			}

			acted := e.actedPredicate(in.SessionID)
			n, ok := nudge.Decide(j, e.nudgeConfig(), acted)
			if !ok {
				return nil // stay silent
			}

			// Record the fired nudge for dedup + budget, then persist.
			j.Fired = append(j.Fired, nudge.FiredNudge{Key: n.Key, Turn: j.Turn})
			if err := store.Save(j); err != nil {
				return err
			}

			var out stopHookOutput
			out.HookSpecificOutput.HookEventName = "Stop"
			out.HookSpecificOutput.AdditionalContext = n.Text
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a Stop event on stdin and emit at most one nudge")
	return cmd
}

// actedPredicate returns a function reporting whether this session has already
// proposed/observed for a signal — the suppress-if-acted rule (spec §4). It
// checks the local index for records authored by sessionID touching the
// signal's path (propose) or targeting the signal's memory (observe).
func (e *env) actedPredicate(sessionID string) func(nudge.Signal) bool {
	mems, err := e.led.Memories() // full model.Memory records (Actor + Scope); the
	if err != nil {               // index projection (IndexedMemory) lacks Actor.
		return func(nudge.Signal) bool { return false }
	}
	obs, _ := e.led.Observations()
	return func(s nudge.Signal) bool {
		if s.Verb == "observe" {
			for _, o := range obs {
				if o.Target == s.Memory && o.Actor.SessionID == sessionID {
					return true
				}
			}
			return false
		}
		for _, m := range mems {
			if m.Actor.SessionID != sessionID {
				continue
			}
			for _, p := range m.Scope.Paths {
				if p == s.Path {
					return true
				}
			}
		}
		return false
	}
}
```

Register in `internal/cli/cli.go`:

```go
		newNudgeCmd(g),
```

- [ ] **Step 4: Record surfaced memories in check-action hook**

In `internal/cli/checkaction.go`, inside `runHook`, after the results are retrieved and before emitting, record each surfaced memory into the journal so the observe signals (Task 4) have a source. Add after line 191 (`now := time.Now().UTC()`):

```go
	if nstore, nerr := e.nudgeStore(); nerr == nil && in.SessionID != "" {
		if j, lerr := nstore.Load(in.SessionID); lerr == nil {
			for _, r := range res {
				drift := false
				for _, d := range r.Drift {
					if d.Note != "" {
						drift = true
					}
				}
				path := ""
				if len(r.Memory.EffectiveScope) > 0 {
					path = r.Memory.EffectiveScope[0]
				}
				j.RecordSurfaced(r.Memory.ID, path, drift)
			}
			_ = nstore.Save(j)
		}
	}
```

> `retrieve.Result.Memory` is an `index.IndexedMemory` (not a `model.Memory`): it has **no `Scope` field** — the path globs live in `EffectiveScope []string` (`internal/index/query.go:23`). There is no matched-action-path field on `Result`, so key the surfaced record off `EffectiveScope[0]`. (Contrast: `actedPredicate` in Task 8 uses `e.led.Memories()`, which returns full `model.Memory` records *with* `Scope.Paths` — that one is correct as written.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestNudgeHook|TestSignalHook' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/nudge.go internal/cli/nudge_test.go internal/cli/cli.go internal/cli/checkaction.go
git commit -m "feat(nudge): tm nudge --hook Stop emitter + record surfaced memories (spec §4, §6.1)"
```

---

## Task 9: End-to-end lifecycle (txtar)

**Files:**
- Create: `e2e/testdata/scripts/nudge.txtar`

- [ ] **Step 1: Write the e2e script**

Create `e2e/testdata/scripts/nudge.txtar`:

```
exec tm init

# Record a fail → edit → pass sequence for one session via the PostToolUse hook.
stdin fail.json
exec tm signal --hook
stdin edit.json
exec tm signal --hook
stdin pass.json
exec tm signal --hook

# The Stop hook now emits a pointed propose nudge for the recovered failure.
stdin stop.json
exec tm nudge --hook
stdout 'tm_propose'
stdout 'failed_attempt'

# A second Stop in the same turn is suppressed by the cooldown.
stdin stop.json
exec tm nudge --hook
! stdout 'tm_propose'

-- fail.json --
{"session_id":"sX","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}
-- edit.json --
{"session_id":"sX","tool_name":"Edit","tool_input":{"file_path":"internal/index/index.go"}}
-- pass.json --
{"session_id":"sX","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}
-- stop.json --
{"session_id":"sX"}
```

> **Verify the txtar `stdin <file>` directive** is supported by this repo's testscript setup (check `e2e/e2e_test.go` for the `Cmds`/`Setup` config). If `stdin` from a file is not wired, add a tiny custom command or inline the JSON via an existing mechanism used by `sync.txtar`/`propose.txtar`. The existing scripts use `exec tm ...` with no stdin, so confirm the harness exposes `testscript`'s built-in `stdin` command (it does by default).

- [ ] **Step 2: Run the e2e suite**

Run: `go test ./e2e/ -run TestScripts/nudge -v`
(If the script runner uses a different test name, run `go test ./e2e/ -run Script -v` and confirm `nudge` is picked up.)
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add e2e/testdata/scripts/nudge.txtar
git commit -m "test(nudge): e2e propose-nudge lifecycle with cooldown suppression (spec §8)"
```

---

## Task 10: Documentation — prd.md deltas

**Files:**
- Modify: `prd.md` (§10 hooks, §149, §537, config section)

- [ ] **Step 1: Update prd.md §10 (hooks)**

Add a subsection documenting the two new hook verbs and the Claude Code event wiring:

```markdown
**PostToolUse hook** runs `tm signal --hook`: records nudge signals (fail→pass,
revert, edit churn, surfaced-but-unobserved, drift-anchor) into a per-session
journal under `.git/tm/nudge`. Silent; never blocks.

**Stop hook** runs `tm nudge --hook`: at turn end, emits at most one
proposing/observing nudge per the anti-spam policy (max 3/session, cooldown 3
turns, suppress-if-already-acted, observe outranks propose). Low-pressure
wording; the verbs stay voluntary.

**UserPromptSubmit** records a prompt marker so the user-intervened signal can
detect edit→prompt→re-edit on the same path.
```

- [ ] **Step 2: Update §149**

Add the near-moment nudge as a third delivery mechanism:

```markdown
1.5. **Near-moment nudge for the voluntary verbs.** Between deterministic
delivery and voluntary recall: a PostToolUse/Stop hook detects memory-worthy
moments and escalates the highest-value ones to pointed `tm_propose`/`tm_observe`
prompts, while the verbs themselves stay voluntary.
```

- [ ] **Step 3: Update §537**

Replace the "agents ignore the tool" mitigation text:

```markdown
**Agents ignore the voluntary verbs** → (1) the SessionStart brief injects the
when-to-remember instructions every session; (2) a near-moment nudge engine
(PostToolUse signal recording + Stop emission) escalates the highest-value
moments to pointed prompts, bounded by an anti-spam budget so it never
manufactures junk proposals.
```

- [ ] **Step 4: Add nudge config to the config section**

Document the `nudge.*` keys (enabled, max_per_session, cooldown_turns,
self_review_every, churn_threshold) with their defaults from Task 1.

- [ ] **Step 5: Verify the whole suite is green**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add prd.md
git commit -m "docs(prd): document the near-moment nudge engine (§10, §149, §537, config)"
```

---

## Self-review notes (for the implementer)

- **Spec coverage (this slice):** journal §2 (Tasks 2–3), six signals §3 (Task 4), policy §4 (Task 5), `signal`/`nudge` verbs §6.1 (Tasks 7–8), surfaced-memory recording §2/§3 (Task 8 step 4), config §7 (Task 1), e2e §8 (Task 9), prd deltas §9 (Task 10). PostToolUse advisory injection, the adapter layer, and the other four harnesses are **out of scope for this slice** — Plans 2–3.
- **Two flagged verifications carried from spec §10**, both isolated to a single render/parse site so they don't ripple: the **Stop-hook context-injection shape** (Task 8, `stopHookOutput`) and the **txtar `stdin` directive** (Task 9). Resolve each against a live payload / the harness config before relying on it.
- **`relPath`** in `signal.go` duplicates the path-normalization already inline in `checkaction.go:166-171`; if you touch both, consider extracting one shared helper in `env.go`. Not required for this slice.
```
