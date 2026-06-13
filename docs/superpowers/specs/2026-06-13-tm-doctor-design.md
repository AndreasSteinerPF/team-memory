# `tm doctor` — environment diagnostics

**Date:** 2026-06-13
**Status:** approved, ready for implementation

## Problem

There is no single command to verify a TeamMemory setup is healthy. A user
dogfooding the tool (or onboarding a teammate) has to manually confirm the
ledger branch exists, the index works, the Claude Code hooks are installed, the
MCP server is registered, and a remote is configured. `prd.md §12.2` / §17 names
`tm doctor` as the command that "validate[s] branch, index, hook installation."

## Goal

A read-only `tm doctor` command that runs a fixed set of checks, prints a clear
per-check report, and exits non-zero only when a *core* check fails — so it is
useful both interactively and in CI.

## Decisions (from brainstorming)

1. **Checks:** ledger branch, local index, Claude Code hooks, `policy.yaml`, MCP
   registration, ledger remote.
2. **Read-only.** No `--fix`. Each non-OK result prints the exact remediation
   command. (Repair can be added later if dogfooding shows a need.)
3. **Exit code:** exit 1 iff any check is `FAIL`. `WARN` and `SKIP` exit 0.

## Non-goals

- No `--fix` / auto-repair.
- No checking of non-Claude agent setups (Cursor/Codex/Gemini hook files); only
  the Claude Code hooks and the repo `.mcp.json` are inspected in v1.
- No network reachability probing beyond the existing `remoteAvailable` heuristic.

## Design

### Command

New file `internal/cli/doctor.go` exposing `newDoctorCmd(g *globalOpts)`,
registered in `internal/cli/cli.go`'s `root.AddCommand(...)`.

`tm doctor` takes no args. It does **not** call `openEnv()` — that helper aborts
when the ledger is missing or the index fails to update, which are the exact
conditions `doctor` must diagnose. `doctor` opens each layer directly and
captures per-check errors.

### Severity model

```go
type severity int
const (
    sevOK severity = iota // ✓ healthy
    sevWarn               // ⚠ optional/integration issue, not broken
    sevSkip               // – not applicable to this environment
    sevFail               // ✗ core broken
)

type checkResult struct {
    name   string
    sev    severity
    detail string // short status, e.g. "initialized (42 memories)"
    hint   string // remediation command, shown indented under a non-OK result
}
```

Exit code: process exits 1 iff any result has `sev == sevFail`.

### Checks (run in this order)

| # | Check | OK | WARN | FAIL | SKIP |
|---|-------|----|------|------|------|
| 1 | Ledger branch | `led.Exists()`; detail = memory count | — | branch missing / not a git repo → hint ``run `tm init` `` | — |
| 2 | Local index | `index.Open` + `idx.Update()` succeed (auto-rebuild counts as OK) | — | open/rebuild errors → hint "delete `.git/tm/index.db` and retry" | ledger branch FAIL |
| 3 | policy.yaml | `led.Policy()` returns data that `policy.Load` parses | absent → "using built-in defaults" | present but invalid YAML → hint "fix policy.yaml on the ledger branch" | ledger branch FAIL |
| 4 | Claude Code hooks | both `claudeHookSpecs` present in `.claude/settings.json` | some/all missing, or settings.json unparsable → hint ``run `tm init` `` | — | no `.claude/` dir |
| 5 | MCP registration | `.mcp.json` has `mcpServers.teammemory` with command `tm` / args `["mcp"]` | missing file or entry → hint with the JSON snippet | — | — |
| 6 | Ledger remote | `ledgerRemote()` resolves via `remoteAvailable()` | none configured → "sync/push disabled (fine for solo use)" | — | — |

Checks 2 and 3 depend on the ledger branch existing; when check 1 is FAIL they
return `sevSkip` with detail "ledger not initialized" rather than erroring.

### Output

Always printed to stdout (even on failure):

```
TeamMemory doctor — <repoDir> (branch: <branch>)

  ✓ Ledger branch      initialized (42 memories)
  ✓ Local index        healthy
  ✓ policy.yaml         valid
  ⚠ Claude Code hooks   SessionStart brief missing
      → run `tm init` to reinstall hooks
  ⚠ MCP registration    teammemory not in .mcp.json
      → add: { "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } } }
  ✓ Ledger remote       origin

2 warnings, 0 failures.
```

Summary line counts WARN and FAIL (SKIP/OK not counted). On any FAIL, the
command sets `SilenceErrors`/`SilenceUsage` and returns a sentinel error so the
process exits 1 without cobra re-printing the report.

### Check functions

Each check is a function returning `checkResult`, so the rendering/exit logic is
trivial and the checks are unit-testable in isolation:

- `checkLedger(led *ledger.Ledger) checkResult`
- `checkIndex(led, gitDir) checkResult`
- `checkPolicy(led) checkResult`
- `checkHooks(repoDir string) checkResult` — reuses `countHookEntries` + `claudeHookSpecs`
- `checkMCP(repoDir string) checkResult`
- `checkRemote(e ...) checkResult` — reuses `ledgerRemote`/`remoteAvailable` logic

The pure-filesystem checks (`checkHooks`, `checkMCP`) take a repo dir and read
their own files, so tests point them at temp dirs.

## Testing

- **Unit (`internal/cli/doctor_test.go`):**
  - `checkHooks`: temp `.claude/settings.json` with both / one / no entries →
    OK / WARN / WARN; no `.claude/` → SKIP.
  - `checkMCP`: temp `.mcp.json` with / without the teammemory entry → OK / WARN;
    missing file → WARN.
  - severity→exit mapping: a result set containing a FAIL yields exit-error; a
    set with only WARN/OK/SKIP yields nil.
- **e2e txtar (`e2e/testdata/scripts/doctor.txtar`, matching `init_plugin.txtar`):**
  - `tm init` then `tm doctor` → all checks OK/SKIP, exit 0.
  - `tm doctor` in an uninitialized repo → ledger branch FAIL, exit 1.

## Docs (same commit, per AGENTS.md)

- `prd.md`: move `tm doctor` out of §12.2 "Nice to Have" (mark shipped); add it
  to the §10.5 CLI command list.
- `README.md`: add a `doctor` line to the Commands block.

## Risks

- The MCP check only inspects the repo-local `.mcp.json`; a user who registered
  the server globally will see a spurious WARN. Acceptable for v1 — WARN is
  non-fatal and the hint is informational. Documented here so it is a known,
  intentional limitation.
