# Cursor & Gemini CLI Adapters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Cursor and Gemini CLI to the harness adapter layer, including their failure-flag fail→pass sensor (no numeric exit code), and ship their packaging artifacts.

**Architecture:** Two new `internal/harness` adapters implementing the `Adapter` interface from Plan 2. Both derive command pass/fail from a **failure flag** instead of an exit code — Cursor via the `postToolUseFailure` event (discriminated by `hook_event_name`), Gemini via `AfterTool`'s `tool_response.error`. No engine changes: the existing edit-between guard in the fail→pass detector makes the failure-flag sensor correct without special dedup (see Task 1 note). Packaging is each ecosystem's native artifact.

**Tech Stack:** Go, cobra, `encoding/json`. Depends on Plan 2's `internal/harness` package and `tm init --harness` dispatch.

This is slice 3 of 3. Slices 1 (nudge engine + Claude Code) and 2 (adapter layer + Codex/Copilot) must be merged first.

**Spec:** `docs/superpowers/specs/2026-06-14-cross-harness-memory-engine-design.md` (§6.4 Cursor, §6.5 Gemini, §7 capability matrix, §10 verification).

---

## File structure

- **Create** `internal/harness/cursor.go` + `internal/harness/cursor_test.go`.
- **Create** `internal/harness/gemini.go` + `internal/harness/gemini_test.go`.
- **Create** `internal/cli/install_cursor.go`, `internal/cli/install_gemini.go`.
- **Modify** `internal/cli/init.go` — add `cursor`/`gemini` cases to the `--harness` switch.
- **Modify** `internal/cli/install_test.go` — Cursor/Gemini artifact assertions.
- **Modify** `docs/verification/cross-harness.md` — Cursor/Gemini verification recipes.
- **Modify** `prd.md` — §6.4/§6.5 wiring, finalize §7 capability matrix.

---

## Task 1: Cursor adapter

**Files:**
- Create: `internal/harness/cursor.go`, `internal/harness/cursor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/harness/cursor_test.go`:

```go
package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCursorFailureEventMarksFailed(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"session_id":"s1","hook_event_name":"postToolUseFailure","command":"pytest"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "pytest" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCursorShellCompletionIsSuccess(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"session_id":"s1","hook_event_name":"afterShellExecution","command":"pytest","output":"..."}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

func TestCursorRenderBlockUsesAgentMessage(t *testing.T) {
	a, _ := harness.Get("cursor")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	out := b.String()
	if !strings.Contains(out, `"permission":"deny"`) || !strings.Contains(out, `"agent_message":"checks"`) {
		t.Errorf("render = %s", out)
	}
}

func TestCursorRenderContextUsesAdditionalContext(t *testing.T) {
	a, _ := harness.Get("cursor")
	var b strings.Builder
	a.Render(harness.PostTool, harness.Decision{Context: "fragile area"}, &b)
	if !strings.Contains(b.String(), `"additional_context":"fragile area"`) {
		t.Errorf("render = %s", b.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harness/ -run TestCursor -v`
Expected: FAIL — unknown harness "cursor".

- [ ] **Step 3: Write the Cursor adapter**

Create `internal/harness/cursor.go`. Cursor uses snake_case fields; the fail half comes from the `postToolUseFailure` event (no exit code); advisory injection is `additional_context`; block is `permission: "deny"` with the reason in `agent_message` (delivered on deny). Spec §6.4.

```go
package harness

import (
	"encoding/json"
	"io"
)

func init() { register(cursor{}) }

type cursor struct{}

func (cursor) Name() string { return "cursor" }

func (cursor) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
		Command       string `json:"command"`
		FilePath      string `json:"file_path"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID,
		Command: raw.Command, FilePath: raw.FilePath,
	}
	if kind == PostTool && raw.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.HookEventName == "postToolUseFailure"
	}
	return ev, nil
}

func (cursor) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			Permission   string `json:"permission"`
			AgentMessage string `json:"agent_message"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		AdditionalContext string `json:"additional_context"`
	}{d.Context})
}
```

> **Why no special double-event dedup is needed.** On a *failing* command Cursor fires both `afterShellExecution` (recorded as success, Failed=false) and `postToolUseFailure` (recorded as a fail, Failed=true). The fail→pass detector only treats `Failed=true` outcomes as the "fail" half, and requires an **edit between** the fail and a later same-signature success — there is no edit between the two synthetic outcomes of a *single* command, so the duplicate never produces a spurious recovery. A genuine recovery (fail → edit → re-run succeeds) still fires correctly. No dedup task required.
>
> **VERIFY (spec §10 item 3):** confirm `afterShellExecution`/`postToolUseFailure` payload field names (`command`, `hook_event_name`) and that `additional_context` injects model-visible text on allow. File-edit pre-blocking depends on whether Cursor's `preToolUse` covers edits (Cursor exposes `afterFileEdit` but no documented `beforeFileEdit`); if not, requirement blocking on file edits is shell-only on Cursor — note it in the installer.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/harness/ -run TestCursor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harness/cursor.go internal/harness/cursor_test.go
git commit -m "feat(harness): Cursor adapter with failure-flag fail sensor (spec §6.4)"
```

---

## Task 2: Gemini CLI adapter

**Files:**
- Create: `internal/harness/gemini.go`, `internal/harness/gemini_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/harness/gemini_test.go`:

```go
package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestGeminiAfterToolErrorMarksFailed(t *testing.T) {
	a, _ := harness.Get("gemini")
	in := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cargo test"},"tool_response":{"error":"exit status 101"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "cargo test" {
		t.Errorf("event = %+v", ev)
	}
}

func TestGeminiAfterToolSuccess(t *testing.T) {
	a, _ := harness.Get("gemini")
	in := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cargo test"},"tool_response":{"error":""}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success, got %+v", ev)
	}
}

func TestGeminiRenderBlock(t *testing.T) {
	a, _ := harness.Get("gemini")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	if !strings.Contains(b.String(), `"decision":"deny"`) || !strings.Contains(b.String(), `"reason":"checks"`) {
		t.Errorf("render = %s", b.String())
	}
}

func TestGeminiRenderContext(t *testing.T) {
	a, _ := harness.Get("gemini")
	var b strings.Builder
	a.Render(harness.PostTool, harness.Decision{Context: "constraint"}, &b)
	if !strings.Contains(b.String(), `"additionalContext":"constraint"`) {
		t.Errorf("render = %s", b.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/harness/ -run TestGemini -v`
Expected: FAIL — unknown harness "gemini".

- [ ] **Step 3: Write the Gemini adapter**

Create `internal/harness/gemini.go`. Gemini's `AfterTool` carries `tool_response.error` (non-empty = failure — no numeric exit code); block via `decision: "deny"` + `reason`; advisory via `hookSpecificOutput.additionalContext`. The shell tool is `run_shell_command`. Spec §6.5.

```go
package harness

import (
	"encoding/json"
	"io"
)

func init() { register(gemini{}) }

type gemini struct{}

func (gemini) Name() string { return "gemini" }

func (gemini) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command  string `json:"command"`
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
		ToolResponse struct {
			Error string `json:"error"`
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
		ev.Failed = raw.ToolResponse.Error != ""
	}
	return ev, nil
}

func (gemini) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}{struct {
		AdditionalContext string `json:"additionalContext"`
	}{d.Context}})
}
```

> **VERIFY (spec §10 item 4):** confirm against the pinned Gemini release tag (schema differs from `main`); confirm `AfterTool.additionalContext` is model-visible (`systemMessage` is user-only and must NOT be used for advisory injection).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/harness/ -run TestGemini -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/harness/gemini.go internal/harness/gemini_test.go
git commit -m "feat(harness): Gemini CLI adapter with error-field fail sensor (spec §6.5)"
```

---

## Task 3: Cursor packaging — `tm init --harness cursor`

**Files:**
- Create: `internal/cli/install_cursor.go`
- Modify: `internal/cli/init.go` (add `cursor` case), `internal/cli/install_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/install_test.go`:

```go
func TestInstallCursorWritesHooksAndRules(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "cursor"); code != 0 {
		t.Fatalf("init --harness cursor exit %d", code)
	}
	hooks, err := os.ReadFile(filepath.Join(repo, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("missing .cursor/hooks.json: %v", err)
	}
	for _, want := range []string{"afterShellExecution", "postToolUseFailure", "tm nudge --hook --harness cursor"} {
		if !strings.Contains(string(hooks), want) {
			t.Errorf("hooks.json missing %q:\n%s", want, hooks)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".cursor", "rules", "teammemory.mdc")); err != nil {
		t.Errorf("missing brief rule: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallCursor -v`
Expected: FAIL — unknown harness "cursor" / no files.

- [ ] **Step 3: Implement the Cursor installer**

Create `internal/cli/install_cursor.go`:

```go
package cli

import (
	"os"
	"path/filepath"
)

// installCursor writes Cursor hook + rule + MCP artifacts (spec §6.4).
func installCursor(repoDir string) error {
	cdir := filepath.Join(repoDir, ".cursor")
	if err := os.MkdirAll(filepath.Join(cdir, "rules"), 0o755); err != nil {
		return err
	}
	hooks := `{
  "version": 1,
  "hooks": {
    "beforeShellExecution": [{ "command": "tm check-action --hook --harness cursor" }],
    "afterShellExecution":  [{ "command": "tm signal --hook --harness cursor" }],
    "postToolUseFailure":   [{ "command": "tm signal --hook --harness cursor" }],
    "afterFileEdit":        [{ "command": "tm signal --hook --harness cursor" }],
    "beforeSubmitPrompt":   [{ "command": "tm signal --hook --harness cursor" }],
    "stop":                 [{ "command": "tm nudge --hook --harness cursor" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(cdir, "hooks.json"), []byte(hooks), 0o644); err != nil {
		return err
	}
	rule := `---
alwaysApply: true
---
# TeamMemory
Before risky work, the PreToolUse hook surfaces relevant memories. When you
discover a non-obvious failure, hidden constraint, fragile area, stale doc, or
undocumented decision, record it with tm_propose. When your work bears on a
memory you were shown, tm_observe to confirm or contradict it (with evidence).
`
	if err := os.WriteFile(filepath.Join(cdir, "rules", "teammemory.mdc"), []byte(rule), 0o644); err != nil {
		return err
	}
	mcp := `{ "mcpServers": { "teammemory": { "type": "stdio", "command": "tm", "args": ["mcp"] } } }
`
	return os.WriteFile(filepath.Join(cdir, "mcp.json"), []byte(mcp), 0o644)
}
```

> Note the brief rule uses `alwaysApply: true` so it loads every session (spec §6.4). File-edit pre-blocking is wired via `afterFileEdit` (post) only; if a live check confirms `preToolUse` covers edits, add a `preToolUse` block for requirement enforcement on edits.

- [ ] **Step 4: Add the `cursor` case to `init`**

In `internal/cli/init.go`, add to the `--harness` switch:

```go
	case "cursor":
		if err := installCursor(e.repoDir); err != nil {
			return err
		}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestInstallCursor -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/install_cursor.go internal/cli/init.go internal/cli/install_test.go
git commit -m "feat(install): tm init --harness cursor packaging (spec §6.4)"
```

---

## Task 4: Gemini packaging — `tm init --harness gemini`

**Files:**
- Create: `internal/cli/install_gemini.go`
- Modify: `internal/cli/init.go` (add `gemini` case), `internal/cli/install_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/install_test.go`:

```go
func TestInstallGeminiWritesExtension(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "gemini"); code != 0 {
		t.Fatalf("init --harness gemini exit %d", code)
	}
	settings, err := os.ReadFile(filepath.Join(repo, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("missing .gemini/settings.json: %v", err)
	}
	for _, want := range []string{"AfterTool", "BeforeTool", "AfterAgent", "tm nudge --hook --harness gemini", "mcpServers"} {
		if !strings.Contains(string(settings), want) {
			t.Errorf("settings.json missing %q:\n%s", want, settings)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallGemini -v`
Expected: FAIL.

- [ ] **Step 3: Implement the Gemini installer**

Create `internal/cli/install_gemini.go`:

```go
package cli

import (
	"os"
	"path/filepath"
)

// installGemini writes Gemini CLI settings (hooks + MCP) and a GEMINI.md note
// (spec §6.5). Gemini reads hooks and mcpServers from .gemini/settings.json.
func installGemini(repoDir string) error {
	gdir := filepath.Join(repoDir, ".gemini")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		return err
	}
	settings := `{
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": {
    "BeforeTool":  [{ "command": "tm check-action --hook --harness gemini" }],
    "AfterTool":   [{ "command": "tm signal --hook --harness gemini" }],
    "BeforeAgent": [{ "command": "tm signal --hook --harness gemini" }],
    "AfterAgent":  [{ "command": "tm nudge --hook --harness gemini" }]
  }
}
`
	if err := os.WriteFile(filepath.Join(gdir, "settings.json"), []byte(settings), 0o644); err != nil {
		return err
	}
	brief := `# TeamMemory
When you discover a non-obvious failure, hidden constraint, fragile area, stale
doc, or undocumented decision, record it with tm_propose. When your work bears
on a memory you were shown, tm_observe to confirm or contradict it (with
evidence).
`
	return os.WriteFile(filepath.Join(repoDir, "GEMINI.md"), []byte(brief), 0o644)
}
```

> If a `GEMINI.md` already exists, append the TeamMemory section rather than overwriting. Implement an append-if-exists check (read, skip if it already contains "# TeamMemory", else append).

- [ ] **Step 4: Add the `gemini` case to `init`**

```go
	case "gemini":
		if err := installGemini(e.repoDir); err != nil {
			return err
		}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestInstallGemini -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/install_gemini.go internal/cli/init.go internal/cli/install_test.go
git commit -m "feat(install): tm init --harness gemini packaging (spec §6.5)"
```

---

## Task 5: Verification recipes + prd finalization

**Files:**
- Modify: `docs/verification/cross-harness.md`, `prd.md`

- [ ] **Step 1: Add Cursor/Gemini verification recipes**

Append to `docs/verification/cross-harness.md` (created in Plan 2):
- Cursor: confirm `afterShellExecution`/`postToolUseFailure` field names; confirm `additional_context` injects on allow; check whether `preToolUse` covers file edits.
- Gemini: confirm against the pinned release tag; confirm `AfterTool.additionalContext` is model-visible.

Each with the echo-hook recipe and the field to grep.

- [ ] **Step 2: Update prd.md §6.4/§6.5 wiring and finalize the §7 matrix**

Record the Cursor and Gemini event mappings and packaging, and mark the capability matrix rows as implemented (with the failure-flag sensor noted for both).

- [ ] **Step 3: Verify the whole suite is green**

Run: `go test ./...`
Expected: PASS across all packages and all five harness adapters.

- [ ] **Step 4: Commit**

```bash
git add docs/verification/cross-harness.md prd.md
git commit -m "docs: Cursor/Gemini verification recipes + finalize capability matrix (spec §6.4, §6.5, §7)"
```

---

## Self-review notes (for the implementer)

- **Spec coverage (this slice):** Cursor §6.4 (Tasks 1, 3), Gemini §6.5 (Tasks 2, 4), capability matrix §7 (Task 5), verification §10 items 3–4 (Task 5). All five harnesses are now implemented.
- **Failure-flag correctness** rests on the edit-between guard (Task 1 note) — do not add a "dedup the double event" task; it isn't needed and would risk dropping genuine recoveries. Keep the note in the adapter so a future reader doesn't re-introduce dedup.
- **Two flagged verifications (Task 5)** are isolated to the Cursor/Gemini `Parse` and installer artifacts: Cursor field names + edit-blocking coverage, Gemini release-tag schema + `additionalContext` visibility.
- **`init` append-not-overwrite:** the Gemini installer must not clobber an existing `GEMINI.md`; the Cursor rule file is TeamMemory-owned so overwrite is fine there.
```
