# Cross-Harness Adapter Layer + Codex & Copilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a harness-neutral adapter layer so the hook engine works beyond Claude Code, add PostToolUse advisory injection (deterministic memory surfacing without the pre-tool gate), and ship the two near-drop-in ports: Codex CLI and Copilot CLI.

**Architecture:** A new `internal/harness` package defines a neutral `Event`/`Decision` model and an `Adapter` interface that translates a specific harness's hook JSON in both directions. The three hook CLI verbs (`check-action`, `signal`, `nudge`) gain a `--harness` flag (default `claude`) and route all wire I/O through the selected adapter; the Claude Code shapes move out of the CLI into a `claude` adapter, leaving the engine logic untouched. Advisory injection moves to the post-tool path (where every harness can inject context) and reuses the journal for dedup. Codex and Copilot adapters + their packaging artifacts complete the slice.

**Tech Stack:** Go, cobra, `encoding/json`. Builds directly on Plan 1's `internal/nudge` and the existing `internal/cli/checkaction.go` hook.

This is slice 2 of 3. Slice 1 (`internal/nudge` + Claude Code hooks) must be merged first. Slice 3 adds Cursor and Gemini.

**Spec:** `docs/superpowers/specs/2026-06-14-cross-harness-memory-engine-design.md` (§2 adapter contract, §5 PreToolUse-block + PostToolUse-inject, §6.2 Codex, §6.3 Copilot, §7 config, §10 verification).

---

## File structure

- **Create** `internal/harness/harness.go` — `Event`, `Decision`, `EventKind`, `Adapter` interface, registry.
- **Create** `internal/harness/claude.go` — Claude Code adapter (the shapes currently inline in the CLI).
- **Create** `internal/harness/claude_test.go` — parse/render round-trips.
- **Create** `internal/harness/codex.go` + `internal/harness/codex_test.go`.
- **Create** `internal/harness/copilot.go` + `internal/harness/copilot_test.go`.
- **Modify** `internal/cli/checkaction.go` — route hook I/O through an adapter; add `--harness`.
- **Modify** `internal/cli/signal.go` — route through adapter; add advisory injection.
- **Modify** `internal/cli/nudge.go` — route through adapter.
- **Modify** `internal/nudge/journal.go` — add `Injected []string` (advisory dedup).
- **Modify** `internal/policy/policy.go` — add `Inject` config (`advisory_max_per_session`).
- **Modify** `internal/cli/init.go` — `--harness` installs the right artifacts.
- **Create** `internal/cli/install_codex.go`, `internal/cli/install_copilot.go` — manifest/config generators.
- **Modify** `prd.md` — §584, §10 per-harness.

---

## Task 1: Harness-neutral event/decision model + Claude adapter

**Files:**
- Create: `internal/harness/harness.go`, `internal/harness/claude.go`, `internal/harness/claude_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/harness/claude_test.go`:

```go
package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestClaudeParsePostToolExitCode(t *testing.T) {
	a, err := harness.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test"},"tool_response":{"exit_code":1}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if ev.SessionID != "s1" || ev.Command != "go test" || !ev.Failed || !ev.HasOutcome {
		t.Errorf("parsed event wrong: %+v", ev)
	}
}

func TestClaudeRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("claude")
	var b bytes.Buffer
	if err := a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "do the checks"}, &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "do the checks") {
		t.Errorf("render missing deny decision: %s", out)
	}
}

func TestClaudeRenderEmptyDecisionWritesNothing(t *testing.T) {
	a, _ := harness.Get("claude")
	var b bytes.Buffer
	if err := a.Render(harness.PostTool, harness.Decision{}, &b); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != "" {
		t.Errorf("empty decision should write nothing, got: %q", b.String())
	}
}

func TestUnknownHarnessErrors(t *testing.T) {
	if _, err := harness.Get("nope"); err == nil {
		t.Error("expected error for unknown harness")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harness/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the neutral model + registry**

Create `internal/harness/harness.go`:

```go
// Package harness translates between TeamMemory's harness-neutral hook
// Event/Decision model and each coding agent's concrete hook wire format
// (spec §2 adapter contract). The engine (internal/nudge, internal/retrieve)
// never sees harness-specific JSON.
package harness

import (
	"fmt"
	"io"
)

// EventKind identifies which hook fired.
type EventKind int

const (
	PreTool EventKind = iota // before a tool runs (block-capable)
	PostTool                 // after a tool ran (carries outcome)
	Stop                     // turn ended
	PromptSubmit             // user submitted a prompt
)

// Event is the neutral hook event. Fields not relevant to a kind are zero.
type Event struct {
	Kind       EventKind
	SessionID  string
	ToolName   string
	Command    string // shell-like tool command
	FilePath   string // edit-like tool target
	Failed     bool   // PostTool: the command/tool failed
	HasOutcome bool   // PostTool: a command ran, so Failed is meaningful
}

// Decision is the neutral hook result. A zero Decision means "do nothing".
type Decision struct {
	Block   bool   // deny the tool (requirement enforcement)
	Reason  string // block reason / required checks
	Context string // advisory context to inject without blocking
}

// Empty reports whether the decision has nothing to emit.
func (d Decision) Empty() bool { return !d.Block && d.Reason == "" && d.Context == "" }

// Adapter translates one harness's hook wire format in both directions.
type Adapter interface {
	Name() string
	Parse(kind EventKind, r io.Reader) (Event, error)
	Render(kind EventKind, d Decision, w io.Writer) error
}

var registry = map[string]Adapter{}

func register(a Adapter) { registry[a.Name()] = a }

// Get returns the adapter for name (e.g. "claude", "codex").
func Get(name string) (Adapter, error) {
	if name == "" {
		name = "claude"
	}
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q", name)
	}
	return a, nil
}

// Names returns the registered harness names (for help text / doctor).
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}
```

- [ ] **Step 4: Write the Claude adapter**

Create `internal/harness/claude.go`:

```go
package harness

import (
	"encoding/json"
	"io"
)

func init() { register(claude{}) }

type claude struct{}

func (claude) Name() string { return "claude" }

func (claude) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
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
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind:      kind,
		SessionID: raw.SessionID,
		ToolName:  raw.ToolName,
		Command:   raw.ToolInput.Command,
		FilePath:  raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.ToolResponse.ExitCode != nil && *raw.ToolResponse.ExitCode != 0
	}
	return ev, nil
}

func (claude) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil // emit nothing; the action proceeds
	}
	type spec struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
	}
	s := spec{HookEventName: eventName(kind)}
	if d.Block {
		s.PermissionDecision = "deny"
		s.PermissionDecisionReason = d.Reason
	} else {
		s.AdditionalContext = d.Context
	}
	return json.NewEncoder(w).Encode(struct {
		HookSpecificOutput spec `json:"hookSpecificOutput"`
	}{s})
}

func eventName(kind EventKind) string {
	switch kind {
	case PreTool:
		return "PreToolUse"
	case PostTool:
		return "PostToolUse"
	case Stop:
		return "Stop"
	case PromptSubmit:
		return "UserPromptSubmit"
	}
	return ""
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/harness/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/harness/
git commit -m "feat(harness): neutral hook Event/Decision model + Claude adapter (spec §2)"
```

---

## Task 2: Route the CLI hooks through the adapter

**Files:**
- Modify: `internal/cli/checkaction.go`, `internal/cli/signal.go`, `internal/cli/nudge.go`

- [ ] **Step 1: Add `--harness` and route `signal`**

In `internal/cli/signal.go`, replace the inline `postHookInput` decode with adapter parsing. Add a `--harness` string flag (default `claude`), then:

```go
	a, err := harness.Get(harnessName)
	if err != nil {
		return err
	}
	ev, err := a.Parse(harness.PostTool, cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("signal hook: %w", err)
	}
	if ev.SessionID == "" {
		return nil
	}
	// ... openEnv, load journal, j.Turn++ ...
	switch {
	case ev.HasOutcome:
		j.RecordCommand(ev.Command, ev.Failed)
	case ev.FilePath != "":
		j.RecordEdit(relPath(e, ev.FilePath))
	}
	return store.Save(j)
```

Delete the now-unused `postHookInput` struct (it lives in the claude adapter now). Add the `harness` import.

- [ ] **Step 2: Route `nudge`**

In `internal/cli/nudge.go`, replace `stopHookInput`/`stopHookOutput` with adapter calls. Parse the Stop event for the session id; after `nudge.Decide` returns a nudge, render it:

```go
	a, err := harness.Get(harnessName)
	if err != nil {
		return err
	}
	ev, err := a.Parse(harness.Stop, cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("nudge hook: %w", err)
	}
	// ... decide as before ...
	return a.Render(harness.Stop, harness.Decision{Context: n.Text}, cmd.OutOrStdout())
```

Delete `stopHookInput`/`stopHookOutput`. The Stop-injection-shape VERIFY note from Plan 1 now lives in `claude.go`'s `Render`.

- [ ] **Step 3: Route `check-action`**

In `internal/cli/checkaction.go`, change `runHook` to take the adapter. Parse with `a.Parse(harness.PreTool, ...)`; build the retrieve query from `ev.FilePath`/`ev.Command`; keep the blocker/context split; emit via `a.Render(harness.PreTool, harness.Decision{Block: ..., Reason: ..., Context: ...}, out)`. Move `hookInput`/`hookOutput`/`hookSpecific` out (deleted — the claude adapter owns them). Keep `buildBlockReason`/`buildContext` as the `Reason`/`Context` string builders.

Add `--harness` to `newCheckActionCmd` and pass the resolved adapter into `runHook`.

> **Unused-import cleanup (will not compile otherwise):** deleting `hookInput`/`hookOutput`/`hookSpecific` removes the only `encoding/json` user in `checkaction.go`, and deleting `postHookInput` removes it from `signal.go` (slice 1). Remove `"encoding/json"` from both files' import blocks (the adapter owns JSON now). Go fails the build on an unused import.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS. There is **no** `checkaction_test.go` in the repo — the Claude wire shapes are covered by slice 1's `e2e/testdata/scripts/nudge.txtar`, `signal_test.go`, and `nudge_test.go`. Those are the regression gate: the `claude` adapter's `Render` must reproduce slice 1's `stopHookOutput`/`hookOutput` shapes exactly (`hookSpecificOutput` with `hookEventName`/`permissionDecision`/`permissionDecisionReason`/`additionalContext`).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/checkaction.go internal/cli/signal.go internal/cli/nudge.go
git commit -m "refactor(cli): route hook I/O through the harness adapter (spec §2)"
```

---

## Task 3: Advisory-inject config + journal dedup field

**Files:**
- Modify: `internal/policy/policy.go`, `internal/policy/policy_test.go`, `internal/nudge/journal.go`, `internal/nudge/journal_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/policy/policy_test.go`:

```go
func TestDefaultInjectConfig(t *testing.T) {
	if policy.Default().Inject.AdvisoryMaxPerSession != 5 {
		t.Errorf("Inject.AdvisoryMaxPerSession = %d, want 5", policy.Default().Inject.AdvisoryMaxPerSession)
	}
}
```

Add to `internal/nudge/journal_test.go`:

```go
func TestMarkInjectedDedups(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	if j.AlreadyInjected("MEM1") {
		t.Fatal("fresh journal should not have MEM1")
	}
	j.MarkInjected("MEM1")
	if !j.AlreadyInjected("MEM1") {
		t.Error("MEM1 should be marked injected")
	}
	j.MarkInjected("MEM1") // idempotent
	if len(j.Injected) != 1 {
		t.Errorf("Injected = %v, want one entry", j.Injected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/policy/ ./internal/nudge/ -run 'TestDefaultInject|TestMarkInjected' -v`
Expected: FAIL — `Inject` / `MarkInjected` undefined.

- [ ] **Step 3: Implement**

In `internal/policy/policy.go`, add the field and struct:

```go
	Nudge                  Nudge                           `yaml:"nudge"`
	Inject                 Inject                          `yaml:"inject"`
```

```go
// Inject configures post-tool advisory memory injection (spec §5, §7).
type Inject struct {
	AdvisoryMaxPerSession int `yaml:"advisory_max_per_session"`
}
```

In `Default()`:

```go
		Inject: Inject{AdvisoryMaxPerSession: 5},
```

In `internal/nudge/journal.go`, add the field to `Journal`:

```go
	Injected    []string     `json:"injected,omitempty"` // advisory memory ids delivered
```

And the methods:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/ ./internal/nudge/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/policy.go internal/policy/policy_test.go internal/nudge/journal.go internal/nudge/journal_test.go
git commit -m "feat(inject): advisory-inject config + journal dedup (spec §5, §7)"
```

---

## Task 4: PostToolUse advisory injection

**Files:**
- Modify: `internal/cli/signal.go`
- Test: `internal/cli/signal_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/signal_test.go`:

```go
func TestSignalHookInjectsAdvisoryForEditedPath(t *testing.T) {
	repo := initRepo(t)
	// Propose an active, low-risk decision scoped to docs/** (activates immediately).
	var o, e bytes.Buffer
	cli.Run([]string{"--repo", repo, "propose", "decision", "--title", "Doc style", "--scope", "docs/**", "--guidance", "Use sentence case"}, strings.NewReader(""), &o, &e)

	// A non-Claude harness edit to docs/x.md should surface the memory as context.
	in := `{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"docs/x.md"}}`
	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "signal", "--hook", "--harness", "codex"}, strings.NewReader(in), &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Doc style") {
		t.Errorf("expected advisory injection for docs/x.md, got: %q", out.String())
	}
}
```

> **Ordering:** this test uses `--harness codex`, which doesn't exist until Task 5 — and you **cannot** fall back to `--harness claude`, because the injection block is deliberately skipped on claude (it injects pre-edit instead). So land **Task 5 before this test goes green**: write the test here (it fails: unknown harness), implement the injection logic in Step 3, then unskip/confirm after Task 5. Equivalently, do Task 5 before Task 4.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestSignalHookInjectsAdvisory -v`
Expected: FAIL — no advisory emitted (injection not implemented).

- [ ] **Step 3: Implement injection in the signal command**

In `internal/cli/signal.go`, after recording the edit and before `store.Save(j)`, when the event is an edit, retrieve advisory memories for the path, dedup, cap, and render them via the adapter:

```go
	var decision harness.Decision
	// Advisory injection runs only on non-Claude harnesses: Claude Code already
	// injects advisory memories PRE-edit via check-action, so injecting again
	// post-edit would double-surface. On claude this block is skipped and the
	// empty Decision renders nothing — signal recording above still happens.
	if a.Name() != "claude" && ev.FilePath != "" {
		rel := relPath(e, ev.FilePath)
		res, rerr := e.engine().Retrieve(retrieve.Query{Paths: []string{rel}})
		if rerr == nil {
			max := e.pol.Inject.AdvisoryMaxPerSession
			var fresh []retrieve.Result
			for _, r := range res {
				// Skip requirements (those are blocked pre-tool, not advised post-tool)
				// and anything already injected this session.
				if r.Memory.Enforcement == model.EnforcementRequirement {
					continue
				}
				if j.AlreadyInjected(r.Memory.ID) {
					continue
				}
				if len(j.Injected) >= max {
					break
				}
				fresh = append(fresh, r)
				j.MarkInjected(r.Memory.ID)
				j.RecordSurfaced(r.Memory.ID, rel, hasDrift(r))
			}
			if len(fresh) > 0 {
				decision.Context = buildContext(fresh)
			}
		}
	}
	if err := store.Save(j); err != nil {
		return err
	}
	return a.Render(harness.PostTool, decision, cmd.OutOrStdout())
```

Add a small helper near the file:

```go
func hasDrift(r retrieve.Result) bool {
	for _, d := range r.Drift {
		if d.Note != "" {
			return true
		}
	}
	return false
}
```

Add imports to `signal.go`: `retrieve`, `model`, `harness` (slice 1's `signal.go` imports none of these). `buildContext` is reused from `checkaction.go` (same `cli` package). `retrieve.Result.Memory` is an `index.IndexedMemory` — use `r.Memory.Enforcement`/`r.Memory.ID` (both exist); do **not** reference `r.Memory.Scope` (it has none — see slice 1 Task 8's note).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestSignalHook -v` (after Task 5 if using the codex harness)
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/signal.go internal/cli/signal_test.go
git commit -m "feat(inject): post-tool advisory memory injection for non-Claude harnesses (spec §5)"
```

---

## Task 5: Codex adapter

**Files:**
- Create: `internal/harness/codex.go`, `internal/harness/codex_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/harness/codex_test.go`:

```go
package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCodexParsePostToolExitCode(t *testing.T) {
	a, _ := harness.Get("codex")
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go build"},"tool_response":{"exit_code":2}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "go build" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCodexRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("codex")
	var b bytes.Buffer
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "run checks"}, &b)
	out := b.String()
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "run checks") {
		t.Errorf("render = %s", out)
	}
}

func TestCodexRenderPostToolContext(t *testing.T) {
	a, _ := harness.Get("codex")
	var b bytes.Buffer
	a.Render(harness.PostTool, harness.Decision{Context: "known constraint"}, &b)
	if !strings.Contains(b.String(), `"additionalContext":"known constraint"`) {
		t.Errorf("render = %s", b.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harness/ -run TestCodex -v`
Expected: FAIL — unknown harness "codex".

- [ ] **Step 3: Write the Codex adapter**

Create `internal/harness/codex.go`. Codex uses the same `hookSpecificOutput.{permissionDecision,permissionDecisionReason,additionalContext}` shape as Claude Code (event names differ but Codex accepts the same field names), and carries the exit code in `tool_response.exit_code` (spec §6.2; **verify** — see Task 8). The implementation mirrors `claude` with Codex's `hookEventName` strings:

```go
package harness

import (
	"encoding/json"
	"io"
)

func init() { register(codex{}) }

type codex struct{}

func (codex) Name() string { return "codex" }

func (codex) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
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
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID, ToolName: raw.ToolName,
		Command: raw.ToolInput.Command, FilePath: raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.ToolResponse.ExitCode != nil && *raw.ToolResponse.ExitCode != 0
	}
	return ev, nil
}

func (codex) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	type spec struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
	}
	s := spec{HookEventName: eventName(kind)}
	if d.Block {
		s.PermissionDecision = "deny"
		s.PermissionDecisionReason = d.Reason
	} else {
		s.AdditionalContext = d.Context
	}
	return json.NewEncoder(w).Encode(struct {
		HookSpecificOutput spec `json:"hookSpecificOutput"`
	}{s})
}
```

> Codex's `tool_response` may carry the exit code at a different path on some versions (spec §10 item 1). If Task 8's live-payload check shows otherwise, adjust only this `Parse` — nothing else depends on the wire shape.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/harness/ -run TestCodex -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harness/codex.go internal/harness/codex_test.go
git commit -m "feat(harness): Codex CLI adapter (spec §6.2)"
```

---

## Task 6: Copilot adapter

**Files:**
- Create: `internal/harness/copilot.go`, `internal/harness/copilot_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/harness/copilot_test.go`:

```go
package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCopilotParsePostToolUseFailureMarksFailed(t *testing.T) {
	a, _ := harness.Get("copilot")
	// Copilot's postToolUseFailure event marks the fail half (no exit code needed).
	in := `{"session_id":"s1","hook_event_name":"postToolUseFailure","tool_name":"shell","tool_input":{"command":"npm test"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "npm test" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCopilotParsePostToolSuccess(t *testing.T) {
	a, _ := harness.Get("copilot")
	in := `{"session_id":"s1","hook_event_name":"postToolUse","tool_name":"shell","tool_input":{"command":"npm test"},"toolResult":{"exitCode":0}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

func TestCopilotRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("copilot")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	if !strings.Contains(b.String(), `"permissionDecision":"deny"`) {
		t.Errorf("render = %s", b.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harness/ -run TestCopilot -v`
Expected: FAIL — unknown harness "copilot".

- [ ] **Step 3: Write the Copilot adapter**

Create `internal/harness/copilot.go`. Copilot pre-tool uses `permissionDecision`/`permissionDecisionReason`; post-tool advisory uses `additionalContext`; the **fail** half comes from the `postToolUseFailure` event (discriminated by `hook_event_name`) or the `toolResult.exitCode` when present (spec §6.3):

```go
package harness

import (
	"encoding/json"
	"io"
)

func init() { register(copilot{}) }

type copilot struct{}

func (copilot) Name() string { return "copilot" }

func (copilot) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
		ToolName      string `json:"tool_name"`
		ToolInput     struct {
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"tool_input"`
		ToolResult struct {
			ExitCode *int `json:"exitCode"`
		} `json:"toolResult"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID, ToolName: raw.ToolName,
		Command: raw.ToolInput.Command, FilePath: raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		switch {
		case raw.HookEventName == "postToolUseFailure":
			ev.Failed = true
		case raw.ToolResult.ExitCode != nil:
			ev.Failed = *raw.ToolResult.ExitCode != 0
		}
	}
	return ev, nil
}

func (copilot) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		AdditionalContext string `json:"additionalContext"`
	}{d.Context})
}
```

> **VERIFY (spec §10 item 2):** confirm a script (non-SDK) Copilot `postToolUse` hook actually receives `additionalContext` on output and the `exitCode`/`postToolUseFailure` on input. If the script path drops `additionalContext`, the packaging (Task 7) must ship the SDK hook variant instead. Adjust only this adapter + the Task 7 artifact.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/harness/ -run TestCopilot -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harness/copilot.go internal/harness/copilot_test.go
git commit -m "feat(harness): Copilot CLI adapter with failure-event fail sensor (spec §6.3)"
```

---

## Task 7: Packaging — `tm init --harness {codex,copilot}`

**Files:**
- Modify: `internal/cli/init.go`
- Create: `internal/cli/install_codex.go`, `internal/cli/install_copilot.go`
- Test: `internal/cli/install_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/install_test.go`:

```go
package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCodexWritesPluginArtifacts(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "codex"); code != 0 {
		t.Fatalf("init --harness codex exit %d", code)
	}
	manifest := filepath.Join(repo, ".codex-plugin", "plugin.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("missing plugin manifest: %v", err)
	}
	for _, want := range []string{"hooks", "PostToolUse", "tm signal --hook --harness codex", "tm nudge --hook --harness codex"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("manifest missing %q:\n%s", want, data)
		}
	}
}
```

> Add `runTMLocal` to `internal/cli/testhelpers_test.go` (the `e2e.runTM` is in a different package). Concretely:
>
> ```go
> func runTMLocal(t *testing.T, repo string, args ...string) int {
> 	t.Helper()
> 	var out, errb bytes.Buffer
> 	return cli.Run(append([]string{"--repo", repo}, args...), strings.NewReader(""), &out, &errb)
> }
> ```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallCodex -v`
Expected: FAIL — `init` does not accept `--harness` / no manifest written.

- [ ] **Step 3: Implement the Codex installer**

Create `internal/cli/install_codex.go`:

```go
package cli

import (
	"os"
	"path/filepath"
)

// installCodex writes the .codex-plugin artifacts that wire TeamMemory's hooks
// and MCP server into Codex CLI (spec §6.2). repoDir is the project root.
func installCodex(repoDir string) error {
	dir := filepath.Join(repoDir, ".codex-plugin")
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		return err
	}
	manifest := `{
  "name": "teammemory",
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": "hooks/hooks.json"
}
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		return err
	}
	hooks := `{
  "PreToolUse":  [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm check-action --hook --harness codex" }] }],
  "PostToolUse": [{ "matcher": "^(Bash|apply_patch)$", "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
  "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "tm signal --hook --harness codex" }] }],
  "Stop": [{ "hooks": [{ "type": "command", "command": "tm nudge --hook --harness codex" }] }]
}
`
	return os.WriteFile(filepath.Join(dir, "hooks", "hooks.json"), []byte(hooks), 0o644)
}
```

> **VERIFY (spec §10 item 1):** the `apply_patch` matcher in `PreToolUse`/`PostToolUse` only fires if Codex emits those hooks for file edits. If Task 8 shows Codex is still Bash-only, change the matcher to `^Bash$` and note in the manifest comment that file-edit retrieval is unavailable until upstream lands.

- [ ] **Step 4: Wire `--harness` into `init`**

In `internal/cli/init.go`, add a `--harness` string flag. `newInitCmd` does **not** build an `env` — its `RunE` computes a local `repoDir, err := filepath.Abs(g.repo)` (`internal/cli/init.go:24`). Use that local `repoDir` variable (there is no `e` in scope). After the existing ledger/hook setup, dispatch:

```go
	switch harnessName {
	case "", "claude":
		// existing Claude Code install path (unchanged)
	case "codex":
		if err := installCodex(repoDir); err != nil {
			return err
		}
	case "copilot":
		if err := installCopilot(repoDir, cmd.OutOrStdout()); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown harness %q", harnessName)
	}
```

> `fmt` is already imported in `init.go`. Slice 3 adds the `cursor`/`gemini` arms to this same switch — match this `repoDir` identifier there too.

- [ ] **Step 5: Implement the Copilot installer**

Create `internal/cli/install_copilot.go` mirroring `installCodex`, writing `.github/hooks/teammemory.json` (Copilot repo-scoped hooks) and `~/.copilot/mcp-config.json` guidance, plus an `AGENTS.md` note. Concretely, write the repo hooks file:

```go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// installCopilot writes Copilot CLI hook artifacts and prints the user-scope
// MCP config the user must add by hand (spec §6.3).
func installCopilot(repoDir string, out io.Writer) error {
	dir := filepath.Join(repoDir, ".github", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hooks := `{
  "version": 1,
  "hooks": {
    "preToolUse":  [{ "type": "command", "bash": "tm check-action --hook --harness copilot" }],
    "postToolUse": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "postToolUseFailure": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "userPromptSubmitted": [{ "type": "command", "bash": "tm signal --hook --harness copilot" }],
    "agentStop": [{ "type": "command", "bash": "tm nudge --hook --harness copilot" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(dir, "teammemory.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, `Copilot MCP: add to ~/.copilot/mcp-config.json →`)
	fmt.Fprintln(out, `  {"mcpServers":{"teammemory":{"type":"local","command":"tm","args":["mcp"]}}}`)
	return nil
}
```

> The MCP config lives in the user's home (`~/.copilot/mcp-config.json`), not the repo — so `init` prints the snippet rather than writing into `$HOME` automatically. The test (`TestInstallCopilot...`, if you add one) should assert the repo hooks file, not the printed line.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestInstall -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/init.go internal/cli/install_codex.go internal/cli/install_copilot.go internal/cli/install_test.go
git commit -m "feat(install): tm init --harness for Codex and Copilot packaging (spec §6.2, §6.3)"
```

---

## Task 8: Live-payload verification harness

**Files:**
- Modify: `internal/cli/doctor.go` (add per-harness checks) OR create `docs/verification/cross-harness.md`
- Test: `internal/cli/doctor_test.go` (if doctor is extended)

- [ ] **Step 1: Decide the mechanism**

These checks require a live harness, so they cannot be pure unit tests. Implement a documented, runnable verification rather than asserting against doc-derived fixtures (spec §10).

- [ ] **Step 2: Add an echo-hook recipe**

Create `docs/verification/cross-harness.md` documenting, per harness, how to dump a real hook payload (an echo hook that writes stdin to a file) and what to confirm:
- Codex: do `PreToolUse`/`PostToolUse` fire for `apply_patch`? Is the exit code at `tool_response.exit_code`?
- Copilot: does a script `postToolUse` hook receive `additionalContext` on output and `exitCode`/`postToolUseFailure` on input?

Include the exact echo-hook JSON/script and the field to grep for, so a human (or a follow-up agent with the harness installed) can confirm and tick the spec §10 boxes.

- [ ] **Step 3: Commit**

```bash
git add docs/verification/cross-harness.md
git commit -m "docs(verify): live-payload verification recipes for Codex/Copilot hooks (spec §10)"
```

---

## Task 9: Documentation — prd.md deltas

**Files:**
- Modify: `prd.md` (§584, §10 per-harness)

- [ ] **Step 1: Update §584**

Reframe hook-first integration from Claude-Code-specific to a shared engine with per-harness adapters; record the one fidelity difference (advisory pre-edit on Claude Code, post-edit elsewhere).

- [ ] **Step 2: Update §10**

Add the per-harness event mapping for Codex and Copilot (PreTool-block + PostTool-inject), the `--harness` flag, and the `tm init --harness` packaging.

- [ ] **Step 3: Verify the whole suite is green**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add prd.md
git commit -m "docs(prd): cross-harness adapter layer + Codex/Copilot (§584, §10)"
```

---

## Self-review notes (for the implementer)

- **Spec coverage (this slice):** adapter contract §2 (Tasks 1–2), PostToolUse advisory inject §5 (Tasks 3–4), Codex §6.2 (Tasks 5, 7), Copilot §6.3 (Tasks 6, 7), config §7 (Task 3), verification §10 (Task 8), prd §584/§10 (Task 9). Cursor + Gemini are Plan 3.
- **The Task 2 refactor is the risk point:** the `claude` adapter must reproduce the exact prior wire shapes or Plan 1's tests break. Run `go test ./...` after Task 2 before proceeding — green there is the gate.
- **Two flagged verifications (Task 8)** are isolated to single adapter `Parse`/packaging sites: Codex `apply_patch` coverage + exit-code path, Copilot script-hook `additionalContext`. Do not let a doc override a live payload.
- **Claude Code double-surfacing:** Task 4 skips post-tool injection on `--harness claude` because that harness already injects pre-edit. Keep that guard.
```
