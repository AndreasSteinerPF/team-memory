# Cross-Harness E2E Test Framework — Design

**Date:** 2026-06-14 (revised 2026-06-15 after independent review)
**Status:** Approved (brainstorming) — decomposed into two implementation plans
**Owner:** Andreas Steiner

## Goal

A durable, extensible end-to-end test framework that pins the real behavior of
every supported coding-agent harness (claude, codex, copilot, cursor, gemini)
against TeamMemory's hook integration. It must be cheap to extend **horizontally**
(add a harness) and **vertically** (add a behavior/scenario), and it must cover
**all current harness-facing functionality**: adapter `Parse`/`Render`, the
`tm … --hook` CLI path, `tm init --harness X` packaging, and full engine
scenarios (fail→pass nudge, requirement block-until-ack, advisory context
injection) driven through each harness's real wire format.

This framework replaces one-off manual verification with re-runnable tests and
resolves the live-payload VERIFY items left open by the cross-harness slices
(prd.md §10.6; the adapter comments at `copilot.go:20`, `cursor.go:23`,
`gemini.go:13`).

## Decisions locked during brainstorming

1. **Coverage:** harness-facing surface **plus** full engine scenarios per harness.
2. **Fixture source:** captured from live CLIs (the capture/live tiers treat all
   five harnesses as required-installed-and-authenticated). Fixtures are real
   recorded payloads — normalized for portability (see decision 6).
3. **Scenario engine:** **capture-once, then deterministic replay.** Live agents
   are driven once to emit real hook payloads recorded to `testdata/<harness>/`;
   scenario tests replay those payloads through the CLI **in-process** and assert
   engine outcomes deterministically. A separate live tier re-confirms firing.
4. **Default gating:** **tiered.** Default `go test ./...` runs contract + replay
   + packaging against committed fixtures (no CLIs). Capture and live-firing
   tiers sit behind `//go:build harness_live` and require all five CLIs.
5. **Extensibility backbone:** table-driven matrix with per-harness descriptors
   (Approach A), on-disk fixtures, assertions on a neutral result.

### Decisions added/corrected after independent review

6. **Replay runs in-process via `cli.Run`,** matching the existing e2e pattern
   (`e2e/helpers_test.go:55` `runTM`), NOT a shelled-out binary. "Real CLI"
   means the agent binary, and applies only to the capture/live tiers.
7. **Fixtures are normalized templates, not byte-exact recordings.** Capture
   rewrites the machine-specific repo root to a `{{REPO}}` placeholder (and pins
   a fixed `session_id`); replay substitutes the temp repo dir back in. This is
   required because `runHook`/`relPath` (`checkaction.go:151-158`,
   `signal.go:140-150`) relativize `file_path` against the repo dir — a raw
   absolute path from another machine escapes the root and silently fails scope
   matching. Decision 3's "exact payloads" means "exact modulo this normalization."
8. **No neutral `Decode` is added.** The `Adapter` interface
   (`internal/harness/harness.go:44-48`) has only `Parse` and `Render`; there is
   no inverse codec and inventing one duplicates `Render`. Instead each descriptor
   exposes small **assertion helpers** that unmarshal the rendered wire shape
   (`IsDeny`, `BlockReason`, `AdvisoryContext`), exactly as today's tests already
   do (`e2e/checkaction_test.go:74-88, 198-211`). Neutral expectations sit on top.
9. **The capability matrix is authored in prd.md §10.6, not §7** (§7 is
   Architecture). §10.6's current table is wire-shape only; the per-harness
   *scenario-capability* matrix is **new content that must be added to prd.md
   §10.6 in the same commit** (AGENTS.md: prd.md is authoritative). The test
   coverage report is a **conformance check against** that table — a mismatch
   fails — not a replacement source of truth.
10. **The recording mechanism is a test-only helper, not a shipped `tm` verb.**
    Adding a stdin-recording subcommand to the production CLI would change
    prd.md §10.5's command list; instead capture uses a standalone gated helper
    under `e2e/harness/cmd/recordhook`. The shipped `tm` surface is unchanged.

## Decomposition into two plans

The work splits cleanly by live-CLI dependency. Plan A lands now with no CLIs and
is fully CI-safe; Plan B is gated and depends on environment readiness (and on
Cursor's CLI, which currently won't start).

### Plan A — Default tiers (lands now, no live CLIs)

Descriptor + scenario + runner framework, plus the three default-gated tiers,
driven by committed fixtures and in-process `cli.Run`. Also adds the Taskfile's
build/test/default-tier targets.

- `descriptor.go`, `scenario.go`, `runner.go`, `capabilities.go`
- `descriptors/*.go` (one per harness)
- `contract_test.go` (Tier 1), `replay_test.go` (Tier 2), `packaging_test.go` (Tier 3)
- Seed `testdata/<harness>/` with fixtures (authored from the adapters' known
  shapes for Plan A; replaced by real captures in Plan B). Each fixture's
  manifest marks provenance `authored` until a capture upgrades it.
  **Caveat (acknowledged circularity):** authored fixtures are hand-derived from
  the same adapter code the tests exercise, so Plan A's Tier 1/Tier 2 passing
  proves the framework is internally consistent — NOT that the wire format
  matches reality. Wire-format correctness is established only when Plan B's
  capture upgrades each fixture to `captured`. Authored fixtures exist solely to
  let the framework land green and be reviewable before any CLI is available.
- prd.md §10.6: add the scenario-capability matrix.

### Plan B — Capture + live tiers (gated, requires all CLIs)

- `e2e/harness/cmd/recordhook/` — the gated recording helper.
- `capture.go` (`//go:build harness_live`) — drives each real CLI once, writes
  normalized fixtures + manifest.
- `live_test.go` (`//go:build harness_live`) — drives each real CLI, asserts the
  hook fired.
- Taskfile live/capture targets.
- Cursor stays in logged skip-state until its CLI runs (open blocker).
- First capture run resolves the §10.6 VERIFY items; authored fixtures are
  upgraded to `captured` and any adapter the real payload contradicts becomes a
  normal bug-fix follow-up.

## Architecture & package layout

A new `e2e/harness/` subtree (existing `e2e/` engine tests stay untouched).
Tiers share one descriptor set and one `testdata/` tree.

```
e2e/harness/
  descriptor.go        # HarnessDescriptor interface + registry (horizontal axis)
  scenario.go          # neutral Scenario model + registry (vertical axis)
  runner.go            # scenarios × harnesses; skips+logs unsupported/uncaptured combos
  capabilities.go      # Capability enum (conformance-checked against prd.md §10.6)

  contract_test.go     # TIER 1: fixture → Parse → assert Event; Decision → Render golden
  replay_test.go       # TIER 2: scenarios × harnesses via fixtures through cli.Run (in-process)
  packaging_test.go    # TIER 3: tm init --harness X → assert config files/schema
  live_test.go         # TIER 4 (//go:build harness_live): drive real CLI, assert hook fired
  capture.go           # //go:build harness_live: capture-once driver → writes testdata

  cmd/recordhook/      # //go:build harness_live: standalone stdin-recording hook helper
  descriptors/         # claude.go, codex.go, copilot.go, cursor.go, gemini.go
  testdata/<harness>/  # fixtures + provenance manifests
```

### Tiers and gating

| Tier | File | Gating | Live CLIs | Purpose |
|------|------|--------|-----------|---------|
| 1 Contract | `contract_test.go` | default | no | Pin wire format: fixture → `Parse` → `Event`; `Decision` → `Render` golden |
| 2 Replay | `replay_test.go` | default | no | Engine scenarios × harnesses via fixtures through **in-process `cli.Run`** |
| 3 Packaging | `packaging_test.go` | default | no | `tm init --harness X` writes correct config files/schema |
| 4 Live | `live_test.go` | `//go:build harness_live` | yes (all five) | Re-confirm the real CLI loads + fires our hook |
| Capture | `capture.go` | `//go:build harness_live` | yes | (Re)generate Tier-1/2 fixtures from live CLIs |

## The two axes

### Horizontal — `HarnessDescriptor` (one per harness)

```go
type HarnessDescriptor interface {
    Name() string                       // "claude" … wraps the existing harness.Adapter
    Capabilities() CapabilitySet        // which scenarios apply (conformance-checked vs §10.6)
    FixtureDir() string                 // testdata/<harness>

    // assertion helpers — unmarshal this harness's rendered wire output.
    // (No inverse codec; these mirror what Render already emits.)
    IsDeny(out []byte) bool
    BlockReason(out []byte) string
    AdvisoryContext(out []byte) string

    Driver() LiveDriver                 // live-only: how to drive the real CLI
    Packaging() []PackagingExpectation  // files tm init --harness X must write
}
```

A new harness = one `descriptors/<harness>.go` + its fixtures.

### Vertical — `Scenario` (one per behavior, harness-agnostic)

```go
type Scenario struct {
    Name     string
    Requires CapabilitySet   // skipped+logged on harnesses lacking these
    Steps    []Step           // ordered; see fixed verb↔kind mapping below
    Expect   Expectation      // neutral assertion built on the descriptor helpers
}

type Step struct {
    Verb    string // "check-action" | "signal" | "signal-prompt" | "nudge"
    Fixture string // testdata/<harness>/<scenario>/<fixture>.json
}
```

**Fixed verb↔kind mapping** (the CLI does not let a step choose an arbitrary
kind for a verb — verified in the command sources):

| Step `Verb` | `tm` invocation | `EventKind` |
|-------------|-----------------|-------------|
| `check-action` | `check-action --hook` | `PreTool` (`checkaction.go:145`) |
| `signal` | `signal --hook` | `PostTool` (`signal.go:34`) |
| `signal-prompt` | `signal --hook --prompt` | `PromptSubmit` (`signal.go:111`) |
| `nudge` | `nudge --hook` | `Stop` |

The runner, per (scenario, harness): creates a temp ledger repo, runs any setup
(`tm propose/approve/observe` to seed memories), then for each step loads
`testdata/<harness>/<scenario>/<fixture>.json`, substitutes `{{REPO}}` →
temp-repo path, and calls
`cli.Run([]string{"--repo", tmp, <verb…>, "--hook", "--harness", X}, stdin, out, err)`
in-process (the `runTM` pattern). `Expectation` asserts on the collected output
via the descriptor's helpers — e.g. `DenyNamingMemoryID`,
`AdvisoryContextContains("downgrade tests")`, `NudgeFiredContaining(…)` — so the
same expectation holds whether the wire shape is Claude's `hookSpecificOutput`
or Copilot's bare fields.

A new behavior = one `Scenario{}` + one fixture per harness; it runs across every
capable harness automatically.

### Capability gating

`CapabilitySet` enumerates capabilities such as `PreToolBlock`,
`PostToolFailureSensor`, `StopNudge`, `PromptSubmit`, `AdvisoryInjection`. A
scenario requiring a capability a harness does not declare is **skipped with a
logged reason**. The runner emits a coverage summary where each
(scenario × harness) cell is one of:

- **run**
- **skipped: capability X not supported**
- **skipped: no fixtures captured yet**

This summary is **conformance-checked against the authoritative prd.md §10.6
capability matrix**: if a descriptor's declared capability disagrees with §10.6,
the test fails. The report does not replace §10.6 — it verifies reality matches it.

**Conformance mechanism (must be implementable, not prose-diffing).** prd.md
§10.6 carries the capability matrix as a single fenced ` ```capability-matrix `
block in a fixed, trivially-parseable format — a pipe table whose first column is
the harness name and whose remaining columns are capability names, each cell
`yes`/`no`:

```capability-matrix
harness  | PreToolBlock | PostToolFailureSensor | StopNudge | PromptSubmit | AdvisoryInjection
claude   | yes          | yes                   | yes       | yes          | no
codex    | yes          | yes                   | yes       | yes          | yes
…
```

The conformance test reads *that fenced block only* (a ~20-line parser keyed on
the fence label, no general markdown parsing), builds a `CapabilitySet` per
harness, and diffs it against each descriptor's `Capabilities()`. Any
disagreement fails. This keeps prd.md the single authoritative source while
making the matrix machine-checkable. Authoring this fenced block in §10.6 is the
prd.md change that lands with Plan A.

## Fixtures, provenance, and capture

### Layout

```
testdata/<harness>/
  manifest.json                # provenance for each fixture
  <scenario>/<fixture>.json    # recorded hook stdin payload (normalized, {{REPO}} placeholder)
  <scenario>/<fixture>.golden  # (contract tier) expected Render output for a Decision
```

### Normalization (decision 7)

Payloads are stored with the repo root replaced by `{{REPO}}` and a fixed
`session_id`, so they replay in any temp repo on any machine. The runner
substitutes the live temp-repo path before piping. Without this, scope-glob
matching silently fails (raw absolute paths escape the replay repo root).

**The fixed `session_id` is one constant shared across *all steps of a
scenario*, not per fixture.** The nudge journal is keyed by `session_id`
(`signal.go:38,50`; `nudge.go:31,43`), so a multi-step scenario
(fail → pass → nudge) only accumulates a journal if every step replays under the
same id. Capture pins this single per-scenario id, and the runner never
re-randomizes it. A per-fixture id would make the `nudge` step load an empty
journal and stay silent (`nudge.go:49-51`), silently passing a broken scenario.

### `manifest.json`

Per fixture: **provenance** (`authored` | `captured`), the harness CLI
**version** captured from (when `captured`), the **capture date**, the
`EventKind`, and the **driving prompt/command**. Anti-drift: a `captured` fixture
is traceable to "codex 0.139.0, captured 2026-06-15, PostToolUse, from
`codex exec …`". Plan A ships `authored` fixtures; Plan B upgrades them to
`captured`.

### Capture mechanism (`capture.go` + `cmd/recordhook`, `//go:build harness_live`)

Per harness, in a throwaway git repo:
1. `tm init --harness X`.
2. Rewrite the installed hook command to invoke the **`recordhook` helper**
   (a standalone test binary, not part of shipped `tm`) which appends its stdin
   to the target fixture file and exits 0. For copilot's `bash`+`powershell`
   entries, both point at the same helper.
3. Drive the real CLI non-interactively (`codex exec
   --dangerously-bypass-approvals-and-sandbox --dangerously-bypass-hook-trust …`,
   `copilot -p … --allow-all-tools`, the gemini/cursor/claude equivalents).
4. Normalize captured payloads (decision 7) and write/update the manifest.

Invoked via the Taskfile (`task capture` / `task capture:<harness>`), never on
the default run.

**Robustness rules (from prior-session findings):**
- The `recordhook` helper **bounds its stdin read with a timeout** — codex can
  hold the hook's stdin open, and an unbounded read hangs the whole run. (The
  in-process default tiers are unaffected: `cli.Run` reads via
  `json.NewDecoder(r).Decode`, which returns at the first complete object.)
- Capture is **idempotent and diff-reviewed**: it writes to `testdata/`, the diff
  is inspected in git before committing, so a wire-format change surfaces as a
  reviewable diff rather than a silent pass.

### Bootstrapping

Until a harness is captured, its `testdata/<harness>/` may hold only `authored`
fixtures (Plan A) or be absent; the runner **skips with a logged reason** rather
than failing. The suite is green from day one and each harness flips to
`captured` as Plan B runs.

## Taskfile

New `Taskfile.yml` (go-task) at the repo root — the single entry point. go-task
is introduced here at the user's request; the default targets are equivalent to
the documented `go test` invocations, so the dependency is convenience, not a
hard requirement. Build/test/default-tier targets land in Plan A; live/capture
targets land in Plan B.

```yaml
version: '3'
tasks:
  build:        { cmds: ['go build ./...'] }
  test:         { desc: 'Default suite (no CLIs needed)', cmds: ['go test ./...'] }
  test:unit:    { cmds: ['go test ./internal/...'] }

  # Harness E2E tiers — default-gated (committed fixtures, no live CLIs)  [Plan A]
  test:harness:           { desc: 'All default harness tiers', cmds: ['go test ./e2e/harness/...'] }
  test:harness:contract:  { cmds: ['go test ./e2e/harness/ -run TestContract'] }
  test:harness:replay:    { cmds: ['go test ./e2e/harness/ -run TestReplay'] }
  test:harness:packaging: { cmds: ['go test ./e2e/harness/ -run TestPackaging'] }

  # Live-gated — REQUIRE all five CLIs installed + authenticated  [Plan B]
  test:harness:live: { desc: 'Live firing tier (needs all harness CLIs)', cmds: ['go test -tags harness_live ./e2e/harness/ -run TestLive'] }
  capture:           { desc: 'Re-capture fixtures for all harnesses', cmds: ['go test -tags harness_live ./e2e/harness/ -run TestCapture'] }
  'capture:*':       { desc: 'Re-capture one harness, e.g. task capture:codex', vars: { H: '{{index .MATCH 0}}' }, cmds: ['go test -tags harness_live ./e2e/harness/ -run TestCapture/{{.H}}'] }

  ci: { desc: 'What CI runs', cmds: [{ task: build }, { task: test }] }
```

**Subtest selectors are real, not fictional:** `TestCapture` (and `TestLive`,
`TestContract`, `TestReplay`) MUST register one `t.Run(harnessName, …)` subtest
per harness, so `-run TestCapture/codex` resolves. The plan specifies this
structure explicitly. The wildcard task key is quoted (`'capture:*'`).

## Tier assertion detail

- **Tier 1 Contract:** table over every fixture → `adapter.Parse` → assert the
  neutral `Event` (kind, command/path, `Failed`/`HasOutcome`); and for each
  `Decision` variant → `adapter.Render` → compare to the `.golden` wire file.
  **Golden determinism:** Render output is canonicalized before compare
  (compact JSON, sorted keys) so field ordering never flakes the test — the
  current adapters already render from structs, not maps, so ordering is stable,
  but the canonicalize step guards against a future map-based adapter. Golden
  files are regenerated with a `-update` test flag and diff-reviewed in git.
- **Tier 2 Replay:** the scenario × harness matrix above, in-process.
- **Tier 3 Packaging:** absorbs today's `internal/cli/install_test.go`
  expectations into each descriptor's `Packaging()`; the duplicated assertions in
  `install_test.go` are **removed** so there is one source of truth. Asserts
  `tm init --harness X` writes the right paths/schema (codex `.codex/hooks.json`
  wrapped under `"hooks"`; copilot entries carrying both `bash` and `powershell`).
  Bonus: a check that each hook command's `--harness` flag help lists all five
  harnesses (the help strings at `checkaction.go:59`, `signal.go:103`,
  `nudge.go:64` are currently stale at `claude, codex, copilot`).
- **Tier 4 Live:** `tm init --harness X` in a temp repo, drive the real CLI,
  assert the hook fired and `tm` was invoked (weaker robust fact — not exact
  nudge text). Catches packaging/discovery bugs.

## Error handling / failure modes

- Missing/authored-only `testdata/<harness>/` → harness **skipped** with a logged
  reason (never a hard fail).
- Malformed/absent fixture for a step a scenario **requires** on a harness that
  **claims** the capability → **hard fail** (a real gap).
- Descriptor capability disagreeing with prd.md §10.6 → **hard fail** (conformance).
- Live tier with a CLI missing/unauthenticated → **fail** with an actionable
  message naming the missing harness.
- `recordhook` stdin-read bounded by timeout; capture writes diff-reviewed.

## Open items / known blockers

1. **Cursor CLI won't start** — a hard prerequisite for capturing Cursor fixtures
   and the Cursor live tier. Cursor stays in logged skip-state until its CLI
   runs. Diagnosis is a follow-up, not part of building the framework.
2. **Live-payload VERIFY items** (§10.6) — Copilot's exact failure field +
   script-hook `additionalContext` visibility; Cursor field names + edit-event
   coverage; Gemini pinned-tag schema + `additionalContext` visibility. These are
   the **acceptance criteria for Plan B's first capture run**.
3. **Authentication** for capture/live is environmental — an operator
   precondition, out of scope for the framework to manage.

## Durable landing in prd.md (AGENTS.md compliance)

This spec lives under `docs/superpowers/specs/` and is **ephemeral** — it is
removed before pushing completed work (AGENTS.md). The durable content that must
graduate into prd.md as the plans land:

- **§10.6:** the new per-harness scenario-capability matrix as the
  ` ```capability-matrix ` fenced block (the conformance source of truth, parsed
  by the conformance test), and an updated note that the harness test suite
  (already referenced at prd.md:476) now exists with its tier structure.
- No new `tm` command graduates — `recordhook` is a test-only helper, so §10.5's
  command list is unchanged.

## What gets absorbed

- The existing `internal/harness/*_test.go` adapter unit tests stay (Tier-1
  adjacent).
- `internal/cli/install_test.go`'s packaging assertions migrate into the
  packaging descriptors and the now-duplicated assertions there are removed.

## Non-goals

- The framework does not manage CLI installation or authentication.
- The default tier does not drive any live agent.
- No change to runtime adapter behavior except where a captured payload proves
  the current adapter wrong (normal bug-fix follow-ups).
