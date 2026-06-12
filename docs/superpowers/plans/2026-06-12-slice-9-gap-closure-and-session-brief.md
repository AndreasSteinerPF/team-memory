# Slice 9 — PRD Gap Closure + Session-Start Briefing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the seven gaps found in the 2026-06-12 PRD audit and add `tm brief` — a session-start briefing injected into agent context via SessionStart hooks, which (as of 2026) exist with context injection in Claude Code, Codex CLI, Copilot CLI, Cursor, Gemini CLI, and Continue CLI.

**Architecture:** All new logic reuses the existing `env` plumbing in `internal/cli`. The briefing is one text builder plus thin per-tool JSON envelopes. The separate-remote feature is a single git config key (`tm.remote`) consulted by sync/fetch/push. Background push mirrors the existing detached background-fetch pattern. Export preambles are a pure-function addition to `internal/export`.

**Tech Stack:** Go 1.26, cobra, existing internal packages. No new dependencies.

**Conventions (from HANDOFF.md):** TDD per task; push directly to `main` when the slice is complete and CI is green on all 3 OSes; commit messages end with the trailer `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` (the model that executed this slice — matches the pre-Slice-9 git history; keep it honest about which model did the work). Subagents: if a test fails because the *plan* is wrong, STOP and report — do not hack the test green.

**Audit traceability:** Task 1 → gaps 3+7 (LICENSE, README tool list). Task 2 → gap 2 (latency budget). Task 3 → trivia (requirement_enforcement key). Task 4 → gap 5 (separate remote). Task 5 → gap 4 (push on propose/observe). Task 6 → gap 1 (export instruction blocks). Tasks 7+8 → session-start briefing (new feature). Task 9 → gap 6 (runnable demo script). Task 10 → PRD amendments + HANDOFF refresh.

---

### Task 1: LICENSE file + README MCP tool-list fix

**Files:**
- Create: `LICENSE`
- Modify: `README.md:180`

- [ ] **Step 1: Create LICENSE**

Write `LICENSE` at the repo root with exactly this content:

```text
MIT License

Copyright (c) 2026 Andreas Steiner

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Fix the README tool list**

In `README.md` line 180, replace:

```markdown
MCP tools: `tm_propose`, `tm_observe`, `tm_check_action`, `tm_list`, `tm_status`.
```

with:

```markdown
MCP tools: `tm_propose`, `tm_observe`, `tm_check_action`, `tm_search`, `tm_status`.
```

- [ ] **Step 3: Commit**

```bash
git add LICENSE README.md
git commit -m "docs: add MIT LICENSE; fix MCP tool list in README (tm_search, not tm_list)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Tighten hook latency budget to 100ms (PRD §10.1)

**Files:**
- Modify: `e2e/bench_test.go:70-71,98,109`

- [ ] **Step 1: Tighten the budget**

In `e2e/bench_test.go`, make three edits:

Replace the comment on lines 70-71:

```go
// TestHookLatency1000 verifies that check-action --hook completes within 150ms
// on a ledger with 1000 memories (PRD §10.1).
```

with:

```go
// TestHookLatency1000 verifies that check-action --hook completes within 100ms
// on a ledger with 1000 memories (PRD §10.1: "under 100ms end-to-end").
```

Replace line 98:

```go
	const budget = 150 * time.Millisecond
```

with:

```go
	const budget = 100 * time.Millisecond
```

Replace the failure message on line 109:

```go
			t.Fatalf("hook run %d/%d took %v (budget 150ms) on a 1000-memory ledger", i+1, runs, elapsed)
```

with:

```go
			t.Fatalf("hook run %d/%d took %v (budget 100ms) on a 1000-memory ledger", i+1, runs, elapsed)
```

- [ ] **Step 2: Run the benchmark test 3 times to verify it passes reliably**

Run (3 times): `go test ./e2e/ -run TestHookLatency1000 -v -count=1`
Expected: PASS each time (or SKIP if the host is loaded — re-run on an idle machine; we need at least one clean PASS).

**If it consistently FAILS at 100ms on an idle machine: STOP and report.** That means the implementation genuinely misses the PRD budget and the fix is performance work, not a test edit — a plan-level decision (optimize vs. amend the PRD) the lead must make.

- [ ] **Step 3: Commit**

```bash
git add e2e/bench_test.go
git commit -m "test: tighten hook latency budget to 100ms per PRD §10.1

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Parse `requirement_enforcement` policy key (PRD §8.1 fidelity)

The PRD's default `policy.yaml` contains `requirement_enforcement: { human_required: true }` but the `Policy` struct never parses it, so a freshly generated policy.yaml is missing a documented key. v1 behavior hard-codes requirement-via-human-only in `internal/derive/enforcement.go` — this task adds config fidelity only, no behavior change.

**Files:**
- Modify: `internal/policy/policy.go`
- Test: `internal/policy/policy_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/policy/policy_test.go` (add `"strings"` to its imports if not present):

```go
func TestRequirementEnforcementDefaultAndRoundTrip(t *testing.T) {
	p := Default()
	if !p.RequirementEnforcement.HumanRequired {
		t.Fatal("default requirement_enforcement.human_required must be true (prd.md §8.1)")
	}
	y, err := DefaultYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "requirement_enforcement:") {
		t.Fatal("DefaultYAML must serialize the requirement_enforcement key")
	}
	p2, err := Load(y)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.RequirementEnforcement.HumanRequired {
		t.Fatal("Load(DefaultYAML()) must preserve human_required=true")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/policy/ -run TestRequirementEnforcementDefaultAndRoundTrip -v`
Expected: FAIL — compile error: `p.RequirementEnforcement undefined`.

- [ ] **Step 3: Implement**

In `internal/policy/policy.go`:

Add the field to `Policy` (after the `Activation` field):

```go
type Policy struct {
	BaseRisk               map[model.MemoryType]model.Risk `yaml:"base_risk"`
	Escalators             Escalators                      `yaml:"escalators"`
	Activation             Activation                      `yaml:"activation"`
	RequirementEnforcement RequirementEnforcement          `yaml:"requirement_enforcement"`
	Retrieval              Retrieval                       `yaml:"retrieval"`
	Sync                   Sync                            `yaml:"sync"`
}
```

Add the type (after the `Tier` type):

```go
// RequirementEnforcement mirrors prd.md §8.1. v1 derivation hard-codes
// requirement-via-human-approve regardless of this value (see
// internal/derive/enforcement.go); the key is parsed so policy.yaml matches
// the PRD's documented default exactly.
type RequirementEnforcement struct {
	HumanRequired bool `yaml:"human_required"`
}
```

In `Default()`, add after the `Activation: ...},` entry:

```go
		RequirementEnforcement: RequirementEnforcement{HumanRequired: true},
```

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/policy/ -v`
Expected: PASS. If `internal/policy/defaultyaml_test.go` compares exact serialized YAML, add the two new lines (`requirement_enforcement:` / `    human_required: true`) to its expected text — the test output shows the diff. Then run `go test ./...` to confirm nothing else asserts the old YAML shape.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/
git commit -m "feat(policy): parse requirement_enforcement key for PRD §8.1 fidelity

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Separate-remote mode as a persisted config field (PRD §7.1)

`tm init --remote X` currently only prints a hint. Persist it as git config `tm.remote` (repo-local, never synced — correct, since remotes are per-clone), and make sync + background fetch consult it.

**Files:**
- Modify: `internal/cli/env.go`, `internal/cli/init.go`, `internal/cli/sync.go`, `internal/cli/checkaction.go:120-130`
- Test: `e2e/remote_test.go` (create)

- [ ] **Step 1: Write the failing e2e test**

Create `e2e/remote_test.go`:

```go
package e2e

import "testing"

// TestInitPersistsSeparateRemote covers prd.md §7.1 separate-remote mode: the
// --remote value is stored as git config tm.remote, and a flagless `tm sync`
// uses it as the push/fetch target.
func TestInitPersistsSeparateRemote(t *testing.T) {
	bare := t.TempDir()
	gitExec(t, bare, "init", "-q", "--bare", "-b", "main")

	dir := newGitRepo(t)
	writeFile(t, dir, "a.txt", "seed")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")

	if _, errOut, code := runTM(t, dir, "", "init", "--remote", bare); code != 0 {
		t.Fatalf("init --remote failed: %s", errOut)
	}

	if got := gitExec(t, dir, "config", "--get", "tm.remote"); got != bare {
		t.Fatalf("tm.remote = %q, want %q", got, bare)
	}

	// Flagless sync must push the ledger branch to the configured remote.
	if _, errOut, code := runTM(t, dir, "", "sync"); code != 0 {
		t.Fatalf("sync failed: %s", errOut)
	}
	gitExec(t, bare, "rev-parse", "--verify", "refs/heads/teammemory") // fails test if absent
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./e2e/ -run TestInitPersistsSeparateRemote -v`
Expected: FAIL at the `config --get tm.remote` assertion (key never written).

- [ ] **Step 3: Implement**

**`internal/cli/env.go`** — add `"os/exec"` and `"strings"` to imports, and append:

```go
// ledgerRemote returns the remote used for ledger fetch/push: the repo-local
// `tm.remote` git config when set (prd.md §7.1 separate-remote mode), else
// "origin". The value may be a remote name or a URL/path — git accepts both.
func (e *env) ledgerRemote() string {
	if out, err := e.git.Run("config", "--get", "tm.remote"); err == nil {
		if v := strings.TrimSpace(out); v != "" {
			return v
		}
	}
	return "origin"
}

// remoteAvailable reports whether remote is usable as a fetch/push target:
// URLs and paths (anything with a separator) are passed to git verbatim;
// bare names must resolve via `git remote get-url`.
func remoteAvailable(e *env, remote string) bool {
	if strings.ContainsAny(remote, "/:\\") {
		return true
	}
	return exec.Command("git", "-C", e.repoDir, "remote", "get-url", remote).Run() == nil
}
```

**`internal/cli/init.go`** — add `"github.com/AndreasSteinerPF/team-memory/internal/git"` to imports. In `newInitCmd`'s `RunE`, after the `led.Init(py)` block succeeds (before opening the index), add:

```go
			if remote != "" {
				if _, err := (git.Runner{Dir: repoDir}).Run("config", "tm.remote", remote); err != nil {
					return err
				}
			}
```

In `printSetup`, replace:

```go
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote configured: %s (run `tm sync --remote %s`).\n", remote, remote)
	}
```

with:

```go
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote stored as git config tm.remote=%s; sync and background fetch/push use it.\n", remote)
	}
```

**`internal/cli/sync.go`** — change the flag default and resolve through config. Replace:

```go
				res, err := e.led.Sync(remote)
```

with:

```go
				if remote == "" {
					remote = e.ledgerRemote()
				}
				res, err := e.led.Sync(remote)
```

and replace the flag registration:

```go
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote (or path) to sync with")
```

with:

```go
	cmd.Flags().StringVar(&remote, "remote", "", "git remote (or URL/path) to sync with (default: git config tm.remote, else origin)")
```

**`internal/cli/checkaction.go`** — in `maybeTriggerFetch`, replace the block from the `// Only start the subprocess...` comment through `cmd.Start()` (lines 120-132) with:

```go
	// Only start the subprocess when a remote is configured. This avoids
	// creating git lock files in repos without a remote (e.g. tests), which
	// would race with temporary-directory cleanup.
	remote := e.ledgerRemote()
	if !remoteAvailable(e, remote) {
		return
	}

	ref := "refs/heads/" + e.branch
	cmd := exec.Command("git", "-C", e.repoDir, "fetch", "--quiet", "--no-tags",
		remote, ref+":"+ref)
	// Start detached — intentionally not calling Wait; parent may exit first.
	_ = cmd.Start()
```

- [ ] **Step 4: Run the test and the full suite**

Run: `go test ./e2e/ -run TestInitPersistsSeparateRemote -v` → PASS
Run: `go test ./...` → PASS (background-fetch gating behavior is unchanged for repos with no remote).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/ e2e/remote_test.go
git commit -m "feat: persist separate-remote mode as git config tm.remote (PRD §7.1)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Best-effort background push on propose/observe (PRD §7.4)

**Files:**
- Create: `internal/cli/push.go`
- Modify: `internal/cli/propose.go`, `internal/cli/observe.go`
- Test: `e2e/push_test.go` (create)

- [ ] **Step 1: Write the failing e2e test**

Create `e2e/push_test.go`:

```go
package e2e

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestProposeTriggersBackgroundPush covers prd.md §7.4: propose pushes the
// ledger branch to the configured remote best-effort in the background.
func TestProposeTriggersBackgroundPush(t *testing.T) {
	bare := t.TempDir()
	gitExec(t, bare, "init", "-q", "--bare", "-b", "main")

	dir := newGitRepo(t)
	writeFile(t, dir, "billing/migrations/seed.sql", "create table t (id int);")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	gitExec(t, dir, "remote", "add", "origin", bare)

	runTM(t, dir, "", "init")
	if _, errOut, code := runTM(t, dir, "", "propose", "failed_attempt",
		"--title", "Billing migrations require downgrade-path tests",
		"--scope", "billing/migrations/**"); code != 0 {
		t.Fatalf("propose failed: %s", errOut)
	}

	// The push is detached; poll the bare remote until the branch arrives.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if exec.Command("git", "-C", bare, "rev-parse", "--verify", "--quiet",
			"refs/heads/teammemory").Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ledger branch never arrived on the remote")
		}
		time.Sleep(100 * time.Millisecond)
	}

	out, err := exec.Command("git", "-C", bare, "ls-tree", "-r", "--name-only",
		"refs/heads/teammemory").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "memories/") {
		t.Fatalf("pushed tree has no memory record:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./e2e/ -run TestProposeTriggersBackgroundPush -v`
Expected: FAIL with "ledger branch never arrived on the remote" (after ~15s).

- [ ] **Step 3: Implement**

Create `internal/cli/push.go`:

```go
package cli

import "os/exec"

// triggerBackgroundPush fires a detached, best-effort `git push` of the ledger
// branch after a local append (prd.md §7.4). It never blocks the command:
// failures (offline, or non-fast-forward because the remote has records we
// haven't merged) are silently ignored — the next `tm sync` reconciles via
// union-merge. Gated on a usable remote, like maybeTriggerFetch, so repos
// without remotes (e.g. tests) spawn no subprocess.
func triggerBackgroundPush(e *env) {
	remote := e.ledgerRemote()
	if !remoteAvailable(e, remote) {
		return
	}
	ref := "refs/heads/" + e.branch
	cmd := exec.Command("git", "-C", e.repoDir, "push", "--quiet", remote, ref+":"+ref)
	// Start detached — intentionally not calling Wait; parent may exit first.
	_ = cmd.Start()
}
```

In `internal/cli/propose.go`, after the `if err := e.idx.Update(); err != nil { return err }` block (line 62-64), insert:

```go
				triggerBackgroundPush(e)
```

In `internal/cli/observe.go`, after its `if err := e.idx.Update(); err != nil { return err }` block (line 59-61), insert the same line:

```go
				triggerBackgroundPush(e)
```

- [ ] **Step 4: Run the test and the full suite**

Run: `go test ./e2e/ -run TestProposeTriggersBackgroundPush -v` → PASS
Run: `go test ./...` → PASS. (Repos without remotes spawn no push process — same gating as fetch — so no other test's temp-dir cleanup can race.)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/push.go internal/cli/propose.go internal/cli/observe.go e2e/push_test.go
git commit -m "feat: best-effort background push on propose/observe (PRD §7.4)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Export instruction preambles (PRD §10.4)

`tm export` currently emits only the memory list. Add flavor-specific instruction text telling agents when to call `tm_check_action` / `tm_propose` / `tm_observe`. After Slice 9 this is the *fallback* surface (session-start hooks are primary), but it must exist for hook-less surfaces (e.g. Continue IDE extensions).

**Files:**
- Modify: `internal/export/export.go`, `internal/cli/export.go:46`
- Test: `internal/export/export_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/export/export_test.go` (ensure `"strings"` is imported):

```go
func TestInstructionsMentionMCPVerbs(t *testing.T) {
	for _, flavor := range []string{"agents", "claude", "cursor"} {
		s := Instructions(flavor)
		for _, verb := range []string{"tm_check_action", "tm_propose", "tm_observe"} {
			if !strings.Contains(s, verb) {
				t.Fatalf("Instructions(%q) missing %s", flavor, verb)
			}
		}
	}
	if !strings.Contains(Instructions("claude"), "PreToolUse") {
		t.Fatal("claude flavor must say edit-time checks are automatic via the hook")
	}
}

func TestMarkdownIncludesInstructions(t *testing.T) {
	out := Markdown(nil, "Project memory (TeamMemory)", Instructions("agents"))
	if !strings.Contains(out, "tm_propose") {
		t.Fatal("Markdown must embed the instruction preamble inside the generated block")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/export/ -v`
Expected: FAIL — compile error: `Instructions` undefined and `Markdown` arity mismatch.

- [ ] **Step 3: Implement**

In `internal/export/export.go`:

Add after the marker constants:

```go
// Instructions returns the flavor-specific usage preamble (prd.md §10.4): when
// agents should call tm_check_action / tm_propose / tm_observe. The "claude"
// flavor notes that edit-time checks are automatic via the PreToolUse hook;
// other flavors instruct a voluntary check before edits.
func Instructions(flavor string) string {
	var b strings.Builder
	b.WriteString("These memories are validated project judgment from prior agent work in this repo. Treat `warning` and `requirement` entries as policy.\n\n")
	if flavor == "claude" {
		b.WriteString("- Edit-time checks run automatically via the TeamMemory PreToolUse hook. Before planning multi-file work, call the `tm_check_action` MCP tool with the target paths.\n")
	} else {
		b.WriteString("- Before editing files, call the `tm_check_action` MCP tool (or `tm check-action --path <file>`) with the paths you are about to change.\n")
	}
	b.WriteString("- When you discover durable project judgment — a non-obvious failure, a hidden constraint, a fragile area, a stale doc, or an undocumented decision — record it with `tm_propose`. Do not record session state, trivia, or facts derivable from the code.\n")
	b.WriteString("- When your work bears on a memory shown to you, react with `tm_observe`: `confirm` with evidence, `contradict` with evidence, `adjust_scope`, or `mark_stale`.\n")
	return b.String()
}
```

Change `Markdown` to accept and embed the preamble — replace the signature and the heading write:

```go
// Markdown renders rows as a generated instruction block. title is the heading;
// instructions is the usage preamble (may be empty).
func Markdown(rows []index.IndexedMemory, title, instructions string) string {
	var b strings.Builder
	b.WriteString(beginMarker + "\n")
	fmt.Fprintf(&b, "## %s\n\n", title)
	if instructions != "" {
		b.WriteString(instructions + "\n")
	}
```

(The rest of the function body — the empty-rows branch, the row loop, the end marker — is unchanged.)

In `internal/cli/export.go`, replace line 46:

```go
				data = []byte(export.Markdown(active, "Project memory (TeamMemory)"))
```

with:

```go
				data = []byte(export.Markdown(active, "Project memory (TeamMemory)", export.Instructions(format)))
```

- [ ] **Step 4: Run package tests, fix remaining call sites, run e2e**

Run: `go build ./...` — any other `export.Markdown` call site (e.g. older assertions in `internal/export/export_test.go`) fails to compile; add `, ""` or `, Instructions("agents")` as the third argument to match each test's intent.
Run: `go test ./internal/export/ ./internal/cli/ -v` → PASS
Run: `go test ./e2e/ -run 'TestScript' -v` → if `e2e/testdata/scripts/export.txtar` asserts block content, the preamble may break a match; update the txtar expectations to also expect the line containing `tm_propose`. Then PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/export/ internal/cli/export.go e2e/testdata/scripts/export.txtar
git commit -m "feat(export): instruction preambles telling agents when to call MCP verbs (PRD §10.4)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: `tm brief` — session-start briefing command

One text builder (live counts + standing instructions), thin per-tool envelopes. Critical property: **a session hook must never break a session** — in a repo with no ledger, `tm brief` exits 0 and prints nothing.

**Files:**
- Create: `internal/cli/brief.go`
- Modify: `internal/cli/cli.go:48-50` (register), `internal/cli/cli.go:2` (package comment count)
- Test: `e2e/brief_test.go` (create)

- [ ] **Step 1: Write the failing e2e tests**

Create `e2e/brief_test.go`:

```go
package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

func initWithMemory(t *testing.T) string {
	t.Helper()
	dir := newGitRepo(t)
	writeFile(t, dir, "a.txt", "seed")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	runTM(t, dir, "", "init")
	// decision = low risk = active immediately, so counts show 1 active.
	if _, errOut, code := runTM(t, dir, "", "propose", "decision",
		"--title", "Ownership of billing moved to platform team"); code != 0 {
		t.Fatalf("propose failed: %s", errOut)
	}
	return dir
}

func TestBriefEmitsCountsAndInstructions(t *testing.T) {
	dir := initWithMemory(t)
	out, errOut, code := runTM(t, dir, "", "brief")
	if code != 0 {
		t.Fatalf("brief failed: %s", errOut)
	}
	for _, want := range []string{"1 active", "tm_check_action", "tm_propose", "tm_observe"} {
		if !strings.Contains(out, want) {
			t.Fatalf("brief output missing %q:\n%s", want, out)
		}
	}
}

func TestBriefFormats(t *testing.T) {
	dir := initWithMemory(t)

	out, _, code := runTM(t, dir, "", "brief", "--format", "copilot")
	if code != 0 {
		t.Fatal("copilot format failed")
	}
	var copilot struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err := json.Unmarshal([]byte(out), &copilot); err != nil || copilot.AdditionalContext == "" {
		t.Fatalf("copilot envelope wrong: %v\n%s", err, out)
	}

	out, _, _ = runTM(t, dir, "", "brief", "--format", "cursor")
	var cursor struct {
		AdditionalContext string `json:"additional_context"`
	}
	if err := json.Unmarshal([]byte(out), &cursor); err != nil || cursor.AdditionalContext == "" {
		t.Fatalf("cursor envelope wrong: %v\n%s", err, out)
	}

	out, _, _ = runTM(t, dir, "", "brief", "--format", "gemini")
	var gemini struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &gemini); err != nil ||
		gemini.HookSpecificOutput.HookEventName != "SessionStart" ||
		gemini.HookSpecificOutput.AdditionalContext == "" {
		t.Fatalf("gemini envelope wrong: %v\n%s", err, out)
	}
}

// TestBriefWithoutLedgerIsSilent: a session hook must never fail or spam a
// session in a repo where tm isn't initialized.
func TestBriefWithoutLedgerIsSilent(t *testing.T) {
	dir := newGitRepo(t)
	out, errOut, code := runTM(t, dir, "", "brief")
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("want silent success, got code=%d out=%q err=%q", code, out, errOut)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./e2e/ -run TestBrief -v`
Expected: FAIL — `unknown command "brief" for "tm"`.

- [ ] **Step 3: Implement**

Create `internal/cli/brief.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// newBriefCmd emits the session-start briefing: live ledger counts plus the
// standing instructions for the voluntary verbs. Designed to run as a
// SessionStart hook in Claude Code (plain text), Codex CLI and Continue CLI
// (plain text / Claude-compatible), and via JSON envelopes for Copilot CLI,
// Cursor, and Gemini CLI.
func newBriefCmd(g *globalOpts) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Emit a session-start briefing for agent hooks (live counts + usage instructions)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				// A session hook must never break or spam a session: in a repo
				// without an initialized ledger, succeed silently.
				return nil
			}
			defer e.close()
			text, err := buildBrief(e)
			if err != nil {
				return nil // same hook-safety rule: degrade to silence
			}
			out := cmd.OutOrStdout()
			switch format {
			case "text", "claude", "codex", "continue":
				_, werr := fmt.Fprint(out, text)
				return werr
			case "copilot":
				return json.NewEncoder(out).Encode(map[string]string{"additionalContext": text})
			case "cursor":
				return json.NewEncoder(out).Encode(map[string]string{"additional_context": text})
			case "gemini":
				return json.NewEncoder(out).Encode(map[string]any{
					"hookSpecificOutput": map[string]string{
						"hookEventName":     "SessionStart",
						"additionalContext": text,
					},
				})
			default:
				return fmt.Errorf("unknown --format %q (want text|claude|codex|continue|copilot|cursor|gemini)", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text | claude | codex | continue | copilot | cursor | gemini")
	return cmd
}

// buildBrief renders the briefing text: one line of live counts, then the
// standing instructions (prd.md §10.1). Kept short on purpose — this lands in
// every session's context.
func buildBrief(e *env) (string, error) {
	rows, err := e.idx.All()
	if err != nil {
		return "", err
	}
	counts := map[model.Status]int{}
	for _, m := range rows {
		counts[m.Status]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "TeamMemory: shared project memory is active in this repo — %d active, %d provisional, %d contested memories.\n",
		counts[model.StatusActive], counts[model.StatusProvisional], counts[model.StatusContested])
	b.WriteString("- Relevant memories are injected automatically when you edit matching files (PreToolUse hook). For multi-file planning — or if this agent has no TeamMemory edit hook — call the tm_check_action MCP tool with the target paths first.\n")
	b.WriteString("- Record durable project judgment with tm_propose when you discover a non-obvious failure, a hidden constraint, a fragile area, a stale doc, or an undocumented decision. Never record session state, trivia, or facts derivable from the code.\n")
	b.WriteString("- When your work bears on a memory you were shown, react with tm_observe: confirm with evidence, contradict with evidence, adjust_scope, or mark_stale.\n")
	if counts[model.StatusProvisional] > 0 {
		b.WriteString("- Provisional memories await independent validation; if your work touches their scope, your confirmation or contradiction matters.\n")
	}
	return b.String(), nil
}
```

In `internal/cli/cli.go`, register the command — replace:

```go
		newMCPCmd(g),
		// Subsequent tasks register their commands here.
```

with:

```go
		newMCPCmd(g),
		newBriefCmd(g),
		// Subsequent tasks register their commands here.
```

and update the package comment (line 2): change `exposes TeamMemory's 13 commands (prd.md §10.5)` to `exposes TeamMemory's 14 commands (prd.md §10.5)`.

- [ ] **Step 4: Run the tests**

Run: `go test ./e2e/ -run TestBrief -v` → PASS (all four tests)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/brief.go internal/cli/cli.go e2e/brief_test.go
git commit -m "feat: tm brief — session-start briefing with per-agent hook envelopes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: `tm init` installs the SessionStart hook (alongside PreToolUse)

Generalize `internal/cli/plugin.go` from one hard-coded hook to a spec list.

**Files:**
- Modify: `internal/cli/plugin.go` (rewrite), `internal/cli/init.go:64-72`
- Test: `internal/cli/plugin_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/cli/plugin_test.go`:

```go
func TestInstallClaudeCodeHooksAddsSessionStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed, err := installClaudeCodeHooks(dir)
	if err != nil || !installed {
		t.Fatalf("install: installed=%v err=%v", installed, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	for _, spec := range claudeHookSpecs {
		if n := countHookEntries(settings, spec); n != 1 {
			t.Fatalf("%s: want 1 entry, got %d", spec.event, n)
		}
	}
	// Idempotent for the full set.
	installed, err = installClaudeCodeHooks(dir)
	if err != nil || installed {
		t.Fatalf("second install: installed=%v err=%v (want false, nil)", installed, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestInstallClaudeCodeHooks -v`
Expected: FAIL — compile error: `installClaudeCodeHooks` / `claudeHookSpecs` undefined.

- [ ] **Step 3: Rewrite plugin.go**

Replace the entire contents of `internal/cli/plugin.go` with:

```go
package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// hookSpec describes one Claude Code hook entry tm installs into
// .claude/settings.json.
type hookSpec struct {
	event   string // key under "hooks": PreToolUse | SessionStart
	matcher string // empty = no matcher field (SessionStart applies to all)
	command string
}

// claudeHookSpecs is everything `tm init` installs (prd.md §10.1): the
// edit-time check hook and the session-start briefing.
var claudeHookSpecs = []hookSpec{
	{event: "PreToolUse", matcher: "Edit|Write|MultiEdit", command: "tm check-action --hook"},
	{event: "SessionStart", matcher: "", command: "tm brief"},
}

// installClaudeCodeHooks writes tm's hook entries to
// <repoDir>/.claude/settings.json. Returns (true, nil) when at least one entry
// was added, (false, nil) when .claude/ doesn't exist or all entries were
// already present.
func installClaudeCodeHooks(repoDir string) (bool, error) {
	claudeDir := filepath.Join(repoDir, ".claude")
	if _, err := os.Stat(claudeDir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	var settings map[string]any
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return false, err
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}

	added := false
	for _, spec := range claudeHookSpecs {
		if countHookEntries(settings, spec) == 0 {
			addHookEntry(settings, spec)
			added = true
		}
	}
	if !added {
		return false, nil
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func countHookEntries(settings map[string]any, spec hookSpec) int {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return 0
	}
	entries, _ := hooks[spec.event].([]any)
	count := 0
	for _, entry := range entries {
		group, _ := entry.(map[string]any)
		if group == nil {
			continue
		}
		matcher, _ := group["matcher"].(string)
		if matcher != spec.matcher {
			continue
		}
		inner, _ := group["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if hm["command"] == spec.command {
				count++
			}
		}
	}
	return count
}

func addHookEntry(settings map[string]any, spec hookSpec) {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	existing, _ := hooks[spec.event].([]any)
	group := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": spec.command,
			},
		},
	}
	if spec.matcher != "" {
		group["matcher"] = spec.matcher
	}
	hooks[spec.event] = append(existing, group)
}
```

- [ ] **Step 4: Update the existing tests and init.go**

In `internal/cli/plugin_test.go`, mechanically update the pre-existing tests:
- every `installClaudeCodeHook(` → `installClaudeCodeHooks(`
- every `hasHookEntry(settings)` → `countHookEntries(settings, claudeHookSpecs[0]) > 0`
- in `TestInstallClaudeCodeHookIdempotent`, replace `if n := countHookEntries(settings); n != 1 {` with `if n := countHookEntries(settings, claudeHookSpecs[0]); n != 1 {`

In `internal/cli/init.go` `printSetup`, replace:

```go
	installed, err := installClaudeCodeHook(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hook: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed PreToolUse hook in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hook already present in .claude/settings.json.")
	}
```

with:

```go
	installed, err := installClaudeCodeHooks(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hooks: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed Claude Code hooks (PreToolUse check + SessionStart brief) in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hooks already present in .claude/settings.json.")
	}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/cli/ -v` → PASS
Run: `go test ./e2e/ -v` → if `e2e/testdata/scripts/init_plugin.txtar` asserts the old "Installed PreToolUse hook" message or old settings.json shape, update its expectations to the new message and to include the SessionStart entry. Then PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/plugin.go internal/cli/plugin_test.go internal/cli/init.go e2e/testdata/scripts/init_plugin.txtar
git commit -m "feat(init): install SessionStart brief hook alongside PreToolUse (PRD §10.1)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Runnable flagship demo script (PRD §12.1 item 12) + CI wiring

**Files:**
- Create: `demo/run.sh`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the script**

Create `demo/run.sh` (LF line endings — the repo's `.gitattributes` governs; verify `git ls-files --eol demo/run.sh` shows `lf` after staging):

```bash
#!/usr/bin/env bash
# TeamMemory flagship demo (prd.md §13): ambient memory validation across
# branches. Creates a throwaway billing-service repo, then walks the full
# lifecycle: propose → provisional → hook caution → independent confirm →
# auto-activate → human approve to requirement → hook blocks → ack → proceed.
set -euo pipefail

step() { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

step "Build tm"
(cd "$ROOT" && go build -o "$WORK/tm" ./cmd/tm)
TM="$WORK/tm"

step "Seed a fake billing-service repo"
REPO="$WORK/billing-service"
mkdir -p "$REPO/billing/migrations"
git -C "$REPO" init -q -b main
git -C "$REPO" config user.email demo@example.com
git -C "$REPO" config user.name "TM Demo"
cat > "$REPO/billing/migrations/2026_add_invoice_state.sql" <<'SQL'
ALTER TABLE invoices ADD COLUMN state TEXT NOT NULL DEFAULT 'open';
SQL
git -C "$REPO" add .
git -C "$REPO" commit -qm "add invoice_state migration"

step "tm init"
"$TM" --repo "$REPO" init

step "Agent A (feature/invoice-state) hits a rollback failure and proposes"
ID=$("$TM" --repo "$REPO" propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --summary "Rollback failed when invoice_state migration lacked a downgrade path." \
  --guidance "Before modifying billing migrations, check rollback behavior and add downgrade-path tests." \
  --scope "billing/migrations/**" \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor "billing/migrations/2026_add_invoice_state.sql@HEAD" \
  --actor claude-code --session session_a --ctx-branch feature/invoice-state | head -n1)
echo "memory: $ID"
"$TM" --repo "$REPO" show "$ID"

step "Agent B (feature/revenue-reporting) opens a related file — the hook fires"
printf '{"session_id":"session_b","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "Agent B reproduces the failure and confirms"
"$TM" --repo "$REPO" observe "$ID" confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --evidence "test_failure:logs/revenue_rollback_failure.log" \
  --actor codex --session session_b --ctx-branch feature/revenue-reporting

step "Auto-activation (high tier + 1 independent confirmation)"
"$TM" --repo "$REPO" show "$ID"

step "Human escalation to requirement"
"$TM" --repo "$REPO" approve "$ID" --enforcement requirement --confidence high

step "Agent C tries to edit a billing migration — the hook BLOCKS"
printf '{"session_id":"session_c","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "Agent C runs the downgrade tests, acks, and retries"
"$TM" --repo "$REPO" ack "$ID" --session session_c --note "downgrade tests pass"
printf '{"session_id":"session_c","tool_name":"Edit","tool_input":{"file_path":"%s"}}' \
  "$REPO/billing/migrations/2026_add_invoice_state.sql" \
  | "$TM" --repo "$REPO" check-action --hook

step "The ledger is plain git"
git -C "$REPO" log --oneline teammemory -- memories/ observations/

step "Demo complete"
```

- [ ] **Step 2: Run it locally (Git Bash on Windows)**

Run: `bash demo/run.sh`
Expected: every step prints; the first Agent-C hook output contains `"permissionDecision":"deny"`; the post-ack hook output does NOT contain `deny`; the git log shows one memory and two observation commits (confirm + approve). If `ack --session` is rejected as an unknown flag, check `internal/cli/ack.go` for the real flag name and fix the script — do not change ack.go.

- [ ] **Step 3: Wire into CI**

In `.github/workflows/ci.yml`, append after the `Test` step:

```yaml
      # The flagship demo (PRD §12.1 item 12) must stay runnable end-to-end.
      - name: Flagship demo script
        if: runner.os != 'Windows'
        run: bash demo/run.sh
```

- [ ] **Step 4: Commit**

```bash
git add demo/run.sh .github/workflows/ci.yml
git commit -m "feat(demo): runnable flagship demo script, wired into CI (PRD §12.1 #12)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Documentation — README session-start section, PRD amendments, HANDOFF refresh

**Files:**
- Modify: `README.md`, `prd.md`, `HANDOFF.md`

- [ ] **Step 1: README — add a "Session-start briefing" section**

In `README.md`, after the MCP setup section (the one ending with the `.mcp.json` snippet around line 178), insert:

````markdown
## Session-start briefing

`tm brief` emits a short briefing — live ledger counts plus standing instructions for `tm_propose` / `tm_observe` / `tm_check_action` — designed to be injected into agent context at session start. `tm init` installs it automatically for Claude Code. In a repo without an initialized ledger it prints nothing and exits 0, so the hook is always safe to install.

All major agent CLIs now support session-start hooks with context injection (snippets abridged — consult each tool's hooks reference):

**Codex CLI** (`.codex/config.toml`; requires a trusted workspace):

```toml
[[hooks.SessionStart]]
command = ["tm", "brief"]
```

**Copilot CLI** (`.github/hooks/teammemory.json`):

```json
{ "version": 1, "hooks": { "sessionStart": [{ "type": "command", "command": "tm brief --format copilot" }] } }
```

**Cursor** (`hooks.json`):

```json
{ "version": 1, "hooks": { "sessionStart": [{ "command": "tm brief --format cursor" }] } }
```

**Gemini CLI** (`settings.json`):

```json
{ "hooks": { "SessionStart": [{ "type": "command", "command": "tm brief --format gemini" }] } }
```

**Continue CLI**: hook schemas are Claude Code-compatible — use the same entry `tm init` writes for Claude Code.
````

Also in README: where the install/quickstart section describes `tm init`, mention it installs *two* hooks (edit-time check + session brief); add one sentence to the demo section: "Or run the whole lifecycle in one command: `bash demo/run.sh`." Add a "Remotes" note where sync is documented: "`tm init --remote <name-or-url>` stores a separate ledger remote as `git config tm.remote` (PRD §7.1); sync, background fetch, and background push all honor it. `propose`/`observe` push in the background best-effort; `tm sync` reconciles when offline or diverged."

- [ ] **Step 2: PRD amendments**

Make these exact edits in `prd.md`:

1. §10.1 (line 368): `The plugin installs two things:` → `The plugin installs three things:`
2. §10.1, after the PreToolUse hook bullet block (after line 375's quoted requirement message), insert:

```markdown
**SessionStart hook**: runs `tm brief` at session start; stdout is injected as session context. The briefing carries live ledger counts plus the standing instructions for the voluntary verbs — deterministic delivery of *when to remember*, not just *what is remembered*.
```

3. §10.4 (lines 393-395): replace the body with:

```markdown
Same MCP server. As of 2026, Codex CLI, Copilot CLI, Cursor, Gemini CLI, and Continue CLI all support session-start hooks with context injection; `tm brief --format <tool>` emits the briefing in each tool's envelope (setup snippets in the README), so the session-start instruction path works everywhere. `tm export` still generates instruction blocks for `AGENTS.md` and `.cursor/rules` — including usage preambles for the three verbs — as a fallback for surfaces without hooks (e.g. IDE extensions). Projections are clearly marked generated artifacts.
```

4. §10.5 (line 399): `Thirteen commands:` → `Fourteen commands:`; in the command list add after the `tm check-action` line:

```text
tm brief         # session-start briefing for agent hooks (live counts + instructions)
```

5. §7.1 separate-remote paragraph (line 178): replace `a single config field (`remote = git@github.com:acme/billing.memory.git`)` with `` a single git config key (`git config tm.remote git@github.com:acme/billing.memory.git`) ``
6. §12.1: add item `14. Session-start briefing (`tm brief`) with per-tool envelope formats; installed as a Claude Code SessionStart hook by `tm init`.`
7. §15 "Agents ignore the tool" (line 527): append to the paragraph: ` Session-start briefing injects the voluntary-verb instructions deterministically in every major agent CLI.`
8. §18 Decisions Locked: add `14. Session-start briefing is a first-class surface: `tm brief`, installed for Claude Code by `tm init`, with envelope formats for Codex, Copilot CLI, Cursor, Gemini CLI, and Continue.`

- [ ] **Step 3: HANDOFF.md refresh**

Update `HANDOFF.md`:
- "Current status": replace the slices 1–5 status with: "**Slices 1–8 plus Slice 9 (PRD gap closure + session-start briefing) are COMPLETE, pushed to `main`, CI green on {ubuntu, macos, windows}.** The 2026-06-12 PRD audit found 7 gaps; all are closed (see `docs/superpowers/plans/2026-06-12-slice-9-gap-closure-and-session-brief.md`)."
- Replace the slice table's note and the "Next step" section with: "**Next step:** MVP complete per PRD §12.1. Remaining roadmap is Phase 2+ (`tm doctor`, release automation/curl install script, `ownership`/`successful_pattern` types). Release automation (GitHub Releases workflow) is the highest-value next item — the README references binary downloads that don't exist yet."
- Keep conventions item 4's commit trailer as `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` (the model that executed the slice; matches the git history — keep attribution honest).
- Update the stale "Slice 5 scope decisions" bullet: async push and background fetch are now implemented (Slice 9 / earlier), so delete those two deferral lines.

- [ ] **Step 4: Full suite + commit**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS.

```bash
git add README.md prd.md HANDOFF.md
git commit -m "docs: session-start briefing in README+PRD; ratify slice-9 decisions; refresh HANDOFF

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Final verification (slice definition of done)

- [ ] `go build ./... && go vet ./... && go test ./...` green locally
- [ ] `bash demo/run.sh` runs end-to-end locally
- [ ] Push to `main`; confirm CI green on ubuntu, macos, AND windows before calling the slice done

### Out of scope for this slice (explicitly)

- Release automation / curl install script (noted in HANDOFF as the next item; PRD §16 distribution)
- Auto-installing hooks for Codex/Copilot/Cursor/Gemini (documented snippets only — their trust models require user action anyway)
- Any change to derived-state behavior
