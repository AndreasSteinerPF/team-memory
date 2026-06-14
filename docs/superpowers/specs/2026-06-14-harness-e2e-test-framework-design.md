# Cross-Harness E2E Test Framework — Design

**Date:** 2026-06-14
**Status:** Approved (brainstorming) — ready for implementation plan
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

This framework replaces one-off manual verification with re-runnable tests
(see the team preference for durable tests over point-in-time checks) and
resolves the live-payload VERIFY items left open by the cross-harness slices.

## Decisions locked during brainstorming

1. **Coverage:** harness-facing surface **plus** full engine scenarios per
   harness (the most thorough option — not just adapter unit contracts).
2. **Fixture source:** **captured from live** CLIs. The framework treats all
   five harnesses as required-installed-and-authenticated for the capture/live
   tiers. Fixtures are real recorded payloads, not authored-from-docs.
3. **Scenario engine:** **capture-once, then deterministic replay.** Live agents
   are driven once to emit real hook payloads recorded in order to
   `testdata/<harness>/`; scenario tests replay those exact payloads through
   `tm … --hook` and assert engine outcomes deterministically. A separate live
   tier re-confirms the hook still fires.
4. **Default gating:** **tiered.** Default `go test ./...` runs contract +
   replay + packaging against committed captured fixtures (no CLIs needed,
   CI-safe). Capture and live-firing tiers sit behind `//go:build harness_live`
   and require all five CLIs installed + authenticated.
5. **Extensibility backbone:** **table-driven matrix with per-harness
   descriptors** (Approach A), borrowing an on-disk fixture convention, with a
   per-harness escape hatch for genuinely harness-specific quirks. Assertions
   run against a **decoded neutral `Result`**, not raw wire JSON.

## Architecture & package layout

A new `e2e/harness/` subtree (existing `e2e/` engine tests stay untouched).
Four tiers share one set of per-harness descriptors and one `testdata/` tree.

```
e2e/harness/
  descriptor.go        # HarnessDescriptor interface + registry (horizontal axis)
  scenario.go          # neutral Scenario model + registry (vertical axis)
  runner.go            # computes scenarios × harnesses, skips+logs unsupported combos
  decode.go            # per-harness output decoders (wire JSON → neutral Result)
  capabilities.go      # Capability enum mirroring prd.md §7 matrix

  contract_test.go     # TIER 1: each fixture → Parse → assert Event; Decision → Render golden
  replay_test.go       # TIER 2: scenarios × harnesses via captured payloads through `tm … --hook`
  packaging_test.go    # TIER 3: `tm init --harness X` → assert config files/schema
  live_test.go         # TIER 4 (//go:build harness_live): drive real CLI, assert hook fired
  capture.go           # //go:build harness_live: capture-once driver → writes testdata

  descriptors/         # one file per harness: claude.go, codex.go, copilot.go, cursor.go, gemini.go
  testdata/<harness>/  # captured real payloads + provenance manifests
```

### Tiers and gating

| Tier | File | Gating | Needs live CLIs | Purpose |
|------|------|--------|-----------------|---------|
| 1 Contract | `contract_test.go` | default | no | Pin wire format: fixture → `Parse` → `Event`; `Decision` → `Render` golden |
| 2 Replay | `replay_test.go` | default | no | Engine scenarios × harnesses via captured payloads through `tm … --hook` |
| 3 Packaging | `packaging_test.go` | default | no | `tm init --harness X` writes correct config files/schema |
| 4 Live | `live_test.go` | `//go:build harness_live` | yes (all five) | Re-confirm the real CLI loads + fires our hook |
| Capture | `capture.go` | `//go:build harness_live` | yes | (Re)generate Tier-1/2 fixtures from live CLIs |

The descriptors and `testdata/` are the single source of truth shared by all
tiers — a captured fixture both pins the contract (Tier 1) and feeds the replay
scenario (Tier 2).

## The two axes

### Horizontal — `HarnessDescriptor` (one per harness)

```go
type HarnessDescriptor interface {
    Name() string                                       // "claude" … reuses harness.Adapter underneath
    Capabilities() CapabilitySet                        // which scenarios apply (mirrors prd.md §7)
    FixtureDir() string                                 // testdata/<harness>
    Decode(kind EventKind, wire []byte) (Result, error) // wire JSON → neutral Result
    Driver() LiveDriver                                 // live-only: how to drive the real CLI non-interactively
    Packaging() []PackagingExpectation                  // files tm init --harness X must write
}
```

A new harness = one `descriptors/<harness>.go` + its captured fixtures.

### Vertical — `Scenario` (one per behavior, harness-agnostic)

```go
type Scenario struct {
    Name     string
    Requires CapabilitySet   // skipped+logged on harnesses lacking these
    Steps    []Step           // ordered: {Cmd: "signal", Kind: PostTool, Fixture: "cmd-fail"} …
    Expect   Expectation      // neutral assertion on the final Result
}
```

A **Step** names a captured fixture and the `tm … --hook` command that consumes
it; the runner loads `testdata/<harness>/<scenario>/<fixture>.json`, pipes it
through the real CLI command in a temp ledger repo, and collects the rendered
output. **Expectation** asserts on the decoded neutral `Result` — e.g.
`NudgeFiredContaining("downgrade tests")`, `DenyNamingMemoryID`,
`AdvisoryContextPresent` — so the same assertion holds whether the wire shape is
Claude's `hookSpecificOutput` or Copilot's bare fields.

A new behavior = one `Scenario{}` + one captured fixture per harness; it runs
across every capable harness automatically.

### Capability gating

`CapabilitySet` enumerates capabilities such as `PreToolBlock`,
`PostToolFailureSensor`, `StopNudge`, `PromptSubmit`, `ContextInjectionVisible`.
A scenario requiring a capability a harness does not declare is **skipped with a
logged reason** — no silent gaps. The runner emits a coverage summary where each
(scenario × harness) cell is one of:

- **run**
- **skipped: capability X not supported**
- **skipped: no fixtures captured yet**

That report is the living, execution-generated version of the prd.md §7
capability matrix.

## Fixtures, provenance, and capture

### Layout

```
testdata/<harness>/
  manifest.json                # provenance for the whole harness
  <scenario>/<fixture>.json    # one real recorded hook stdin payload
  <scenario>/<fixture>.golden  # (contract tier) expected Render output for a Decision
```

### `manifest.json`

Per fixture, records: the harness CLI **version** captured from, the **capture
date**, the `EventKind`, and the **driving prompt/command** used. This is the
anti-drift mechanism — every fixture is traceable to "codex 0.139.0, captured
2026-06-14, PostToolUse, from `codex exec …`". It is what prevents a recurrence
of the `4d2ecbc` packaging bugs (doc said one thing, reality another).

### Capture mechanism (`capture.go`, `//go:build harness_live`)

Per harness, in a throwaway git repo:
1. `tm init --harness X`.
2. Install a **recording shim** as the hook command — a tiny `tm` mode that
   appends its stdin to the target fixture file and exits 0.
3. Drive the real CLI with the scenario's prompt (non-interactively:
   `codex exec --dangerously-bypass-approvals-and-sandbox
   --dangerously-bypass-hook-trust …`, `copilot -p … --allow-all-tools`,
   the gemini/cursor/claude equivalents).
4. Write/update the manifest.

Invoked via the Taskfile (`task capture` / `task capture:<harness>`), never on
the default run.

**Robustness rules (from prior-session findings):**
- The recording shim **bounds its stdin read with a timeout** — codex can hold
  the hook's stdin open, and an unbounded `ReadToEnd()` hangs the whole run.
- Capture is **idempotent and diff-reviewed**: it writes to `testdata/`, the
  diff is inspected in git before committing, so a harness changing its wire
  format surfaces as a reviewable diff rather than a silent pass.

### Bootstrapping

Capture requires the live CLIs. Until a harness is captured, its
`testdata/<harness>/` is absent and the runner **skips that harness with a
logged "no fixtures captured" reason** rather than failing. The framework lands
green immediately and each harness flips to covered as it is captured.

## Taskfile

New `Taskfile.yml` (go-task) at the repo root — the single entry point, also
absorbing the common Go targets.

```yaml
version: '3'
tasks:
  build:        { cmds: ['go build ./...'] }
  test:         { desc: 'Default suite (no CLIs needed)', cmds: ['go test ./...'] }
  test:unit:    { cmds: ['go test ./internal/...'] }

  # Harness E2E tiers — default-gated (committed fixtures, no live CLIs)
  test:harness:           { desc: 'All default harness tiers', cmds: ['go test ./e2e/harness/...'] }
  test:harness:contract:  { cmds: ['go test ./e2e/harness/ -run Contract'] }
  test:harness:replay:    { cmds: ['go test ./e2e/harness/ -run Replay'] }
  test:harness:packaging: { cmds: ['go test ./e2e/harness/ -run Packaging'] }

  # Live-gated — REQUIRE all five CLIs installed + authenticated
  test:harness:live:   { desc: 'Live firing tier (needs all harness CLIs)', cmds: ['go test -tags harness_live ./e2e/harness/ -run Live'] }
  capture:             { desc: 'Re-capture fixtures for all harnesses', cmds: ['go test -tags harness_live ./e2e/harness/ -run Capture'] }
  capture:*:           { desc: 'Re-capture one harness, e.g. task capture:codex', vars: { H: '{{index .MATCH 0}}' }, cmds: ['go test -tags harness_live ./e2e/harness/ -run Capture/{{.H}}'] }

  ci:           { desc: 'What CI runs', cmds: [{ task: build }, { task: test }] }
```

Capture and live are build-tagged Go tests run via `-run`, so there is no
separate binary. The default `task test` and `task ci` never touch a live CLI.

## Tier assertion detail

- **Tier 1 Contract:** table over every fixture → `adapter.Parse` → assert the
  neutral `Event` (kind, command/path, `Failed`/`HasOutcome`); and for each
  `Decision` variant → `adapter.Render` → compare to the `.golden` wire file.
  Pure wire-format pin.
- **Tier 2 Replay:** the scenario × harness matrix described above.
- **Tier 3 Packaging:** absorbs and extends today's
  `internal/cli/install_test.go` expectations into each descriptor's
  `Packaging()` — asserts `tm init --harness X` writes the right paths with the
  right schema (e.g. codex `.codex/hooks.json` wrapped under `"hooks"`; copilot
  entries carrying both `bash` and `powershell` keys).
- **Tier 4 Live:** `tm init --harness X` in a temp repo, drive the real CLI,
  assert the hook fired and `tm` was invoked (weaker, robust fact — not exact
  nudge text). Catches packaging/discovery bugs.

## Error handling / failure modes

- Missing `testdata/<harness>/` → harness **skipped** with a logged reason
  (never a hard fail) so the suite is green from day one and fills in as
  captures land.
- Malformed/absent fixture for a step a scenario **requires** on a harness that
  **claims** the capability → **hard fail** (a real gap, not an expected skip).
- Live tier with a CLI missing/unauthenticated → **fail** with an actionable
  message naming the missing harness (the "REQUIRE all installed" contract for
  that tier).
- Capture stdin-read bounded by timeout (codex hang fix); capture writes are
  diff-reviewed before commit.

## Open items / known blockers

1. **Cursor CLI won't start** — a hard prerequisite for capturing Cursor
   fixtures and for the Cursor live tier. Cursor stays in logged skip-state
   until its CLI runs. Diagnosis is a follow-up task, not part of building the
   framework.
2. **Live-payload VERIFY items** carried from the cross-harness slices —
   Copilot's exact failure field + script-hook `additionalContext`
   visibility; Cursor field names + edit-event coverage + `additional_context`
   visibility; Gemini pinned-tag schema + `additionalContext` visibility. These
   are the **acceptance criteria for the first capture run** — capturing real
   payloads resolves them.
3. **Authentication** for the capture/live tiers is environmental (each CLI must
   be logged in) — an operator precondition, out of scope for the framework to
   manage.

## What gets absorbed

- The existing `internal/harness/*_test.go` adapter unit tests stay (Tier-1
  adjacent).
- `internal/cli/install_test.go`'s packaging assertions migrate into the
  packaging descriptors so there is one source of packaging truth.

## Non-goals

- The framework does not manage CLI installation or authentication.
- The default tier does not attempt to drive any live agent.
- No change to runtime adapter behavior except where a captured payload proves
  the current adapter wrong (those become normal bug-fix follow-ups).
