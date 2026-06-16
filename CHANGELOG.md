[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html

# Changelog

All notable changes to TeamMemory are documented here. The format is based on
[Keep a Changelog], and this project adheres to [Semantic Versioning].

## [0.3.0] - 2026-06-16

The "MCP everywhere, merge-safely" release. `tm init` now registers the
`teammemory` MCP server **automatically for all five harnesses** instead of
printing manual instructions for some of them — and every write merges into
existing config rather than clobbering it, so re-running is safe.

### Added

- **Automatic MCP registration for every harness.** `tm init` (default `claude`)
  registers the `teammemory` MCP server in the repo-root `.mcp.json`;
  `tm init --harness {codex,copilot,cursor,gemini}` registers it in that agent's
  MCP config — Codex appends an `[mcp_servers.teammemory]` table to
  `~/.codex/config.toml`, Copilot merges `~/.copilot/mcp-config.json`, Cursor
  merges `.cursor/mcp.json`, and Gemini merges `.gemini/settings.json`. Previously
  Claude/Codex/Copilot only *printed* manual setup snippets. Codex and Copilot
  write into the user's home directory because that is where those CLIs read MCP
  config; every other artifact stays repo-local. (`prd.md §10.6`)
- **Merge-safe, idempotent registration.** Registration reads existing config and
  inserts only the `teammemory` entry — existing MCP servers, hooks, and other
  top-level keys are preserved, and re-running `tm init` is a no-op. Two new
  helpers (`ensureMCPServerJSON` for JSON configs, `ensureCodexMCP` for Codex
  TOML) back this; the packaging-tier E2E suite asserts each harness's MCP target
  with an isolated `$HOME`. (`prd.md §10.6`)

### Changed

- **`tm doctor`** MCP-registration remediation now points at `tm init` (which
  performs the registration) instead of a manual JSON snippet. (`prd.md §10.5`)
- **Cursor and Gemini MCP writes are now merge-safe.** `tm init --harness cursor`
  and `--harness gemini` previously overwrote `.cursor/mcp.json` /
  `.gemini/settings.json` wholesale, discarding any hand-added servers, hooks, or
  keys; both now merge. (`prd.md §10.6`)
- Cross-harness enforcement docs and the live-behavior test tier expanded:
  requirement-blocking is live-verified on Copilot, Cursor, and Gemini, and the
  README enforcement table is aligned with the `prd.md §10.6` capability matrix.

### Fixed

- **Codex path-scoped blocking.** The Codex adapter now parses the `apply_patch`
  tool's file path from the hook payload, so path-scoped `requirement` memories
  correctly block matching Codex edits (previously the path wasn't extracted, so
  path scopes didn't match). (`prd.md §10.6`)

## [0.2.0] - 2026-06-16

The cross-harness + ambient-nudging release. v0.1.x was Claude-only with
deterministic edit-time injection and blocking; v0.2.0 extends the engine to
**five coding agents**, adds a **proactive nudge engine**, **command-scoped
memories** (enforced at Bash time), and an environment doctor — then pins all of
it with an extensible end-to-end test suite validated against the live CLIs.

### Added

- **Cross-harness support — Codex, Copilot, Cursor, and Gemini** (in addition to
  Claude Code). A harness-neutral `Event`/`Decision` model (`internal/harness`)
  with a thin per-agent adapter parses each CLI's concrete hook payload and renders
  decisions back into its wire format; the engine never sees harness-specific JSON.
  `tm init --harness {codex,copilot,cursor,gemini}` writes each agent's hook + MCP
  packaging. Requirement enforcement (PreToolUse block + ack) and advisory memory
  injection work on all five; advisory memories inject **pre-edit** on Claude Code
  and **post-edit** on the others (`tm signal`). Authoritative capability matrix in
  `prd.md §10.6`. (`prd.md §10.6`, `§18`)
- **Near-moment nudge engine.** TeamMemory now *proposes* memories at the moment
  friction happens, not just retrieves them. A per-session journal
  (`.git/tm/nudge/<session>.json`, local-only) records PostToolUse signals
  (`tm signal`) and UserPromptSubmit markers; on Stop (`tm nudge`) the engine
  detects patterns — a fail → fix → pass loop, or a user redirecting the agent
  mid-edit — and surfaces a low-pressure "want to record this?" nudge. Anti-spam
  policy with priority, per-session budget, and cooldown; configurable in
  `policy.yaml`. (`prd.md §10.1`)
- **Command-scoped memories & Bash-time enforcement.** Memories can now scope to
  **command patterns**, not just file paths. Token-aware matching (leading
  subcommand tokens match literally, a trailing `*` matches the rest; flags are
  ignored) — e.g. `pytest *` matches `pytest -q tests/`. The PreToolUse hook
  matches `Bash` actions and blocks unacknowledged `requirement` commands, and a
  structural command channel feeds retrieval. `tm propose`/`tm observe`'s
  `adjust_scope` and `tm_check_action` accept command scopes; bare-binary patterns
  escalate risk. (`prd.md §8.1`, `§9.1`, `§10.1`, `§10.3`, `§11`)
- **`tm doctor`** — environment diagnostics that validate the ledger branch, local
  index, `policy.yaml`, sync remote, installed hooks, and MCP registration, with a
  severity model and a meaningful exit code. (`prd.md §10.5`, `§12.2`)
- **Harness E2E test framework** (`e2e/harness/`). A matrix-driven suite,
  extensible on both axes (add a harness = one descriptor + fixtures + a matrix
  row; add a scenario = one registration). Deterministic default tiers run in CI on
  committed fixtures — **contract** (parse + render goldens), **replay** (engine
  scenarios), **packaging** (`tm init`) — plus a conformance check that fails if a
  descriptor disagrees with the `prd.md §10.6` capability matrix. A
  build-tag-gated (`harness_live`) overlay drives the real CLIs: live hook-firing,
  payload capture/normalization, real-tm behavior tests (requirement block,
  outcome recording), and live failure-sensing.

### Changed

- **Capability matrix is authoritative and live-verified.** Command-failure sensing
  (the fail → fix → pass nudge) works on **Copilot, Cursor, and Gemini** but **not
  Claude Code or Codex**: both fire `PostToolUse` only on tool *success*, so a
  failed command is never observed (verified live, Claude 2.1.177 / Codex 0.139.0).
  Those two degrade gracefully — the nudge stays silent rather than misfiring. Slated
  for re-check by ~2026-08-15 in case either CLI starts emitting a failure event.
  Advisory-context model-visibility on Copilot and Gemini was confirmed by live
  probe (keep injected advisory text descriptive, not imperative — Copilot flags
  imperative hook side-channel instructions as injection).
- **Guidance excludes system/OS-specific memories** so machine-local noise does not
  enter the shared ledger. (`prd.md §5.1`, `§10.3`)

### Fixed

- **Concurrent sync race.** `tm sync` retries on a lost concurrent-push race instead
  of failing, so simultaneous proposals from different clones converge reliably.
- **Cross-harness wire-shape corrections** (caught by live validation before
  release, so the adapters ship correct): Gemini's hook config requires the nested
  `{matcher, hooks[]}` group shape (a flat entry is silently rejected at load);
  Cursor on Windows prepends a UTF-8 BOM to hook stdin that Go's JSON decoder
  rejected — silently breaking every Cursor hook — now stripped for all adapters; a
  failed Cursor command is read from the nested `tool_input.command`; Codex's
  successful `PostToolUse` carries `tool_response` as a string; Copilot and Gemini
  report a command's exit status inside their result text, not a structured field.

## [0.1.1] - 2026-06-13

### Changed

- **Critical-risk memories now auto-activate.** A `critical` memory activates
  once it has **2 independent confirmations** (a stricter bar than the single
  confirm `high` needs), instead of requiring a human `approve`. Auto-enforcement
  is capped at `warning`; `requirement` remains reachable only via human
  `approve`, so agents still cannot create a binding rule (`prd.md §8`).
- Adds an optional per-tier `min_independent_confirms` knob to `policy.yaml`
  (`activation.tiers`); omitted tiers default to 1, so low/medium/high are
  unchanged.

## [0.1.0] - 2026-06-13

First usable release — the complete MVP (`prd.md §12.1`). Suitable for
dogfooding on real repositories.

### Added

- **Git-backed ledger.** Append-only orphan `teammemory` branch storing YAML
  memory and observation records as ULID-named files. No code-branch pollution;
  the full history is auditable with `git log teammemory`.
- **Deterministic derived state.** Status, risk, confidence, and enforcement are
  computed from the ledger and `policy.yaml` — never stored, never agent-settable.
- **Five memory types** — `failed_attempt`, `constraint`, `fragile_area`,
  `stale_doc`, `decision` — each with summary and guidance.
- **Evidence-validated lifecycle.** Memories activate only on independent
  confirmation (different session); contradictions move them to `contested`;
  only a human `approve` can set a `requirement`.
- **Claude Code plugin.** `PreToolUse` hook injects matching memories at edit
  time and blocks unacknowledged `requirement` edits (<100ms, local index, no
  network); `SessionStart` briefing; MCP registration. Installed by `tm init`.
- **MCP server** with five tools: `tm_check_action`, `tm_propose`, `tm_observe`,
  `tm_search`, `tm_status`.
- **Session-start briefing** (`tm brief`) with per-tool envelopes for Claude
  Code, Codex, Copilot, Cursor, Gemini, and Continue CLIs.
- **Export projections** (`tm export`) for `AGENTS.md`, `CLAUDE.md`,
  `.cursor/rules`, and JSON — spliced into existing files without clobbering
  hand-authored content.
- **Union-merge sync.** Concurrent proposals from different clones never
  conflict; opportunistic background fetch keeps memories flowing without manual
  `tm sync`. Supports a separate ledger remote.
- **Requirement acknowledgment** (`tm ack`) — session-scoped, local-only,
  never committed to the ledger.
- **CLI** — `init`, `sync`, `check-action`, `brief`, `propose`, `observe`,
  `ack`, `approve`, `reject`, `list`, `show`, `search`, `export`, `status`,
  `version`.
- **Distribution** via `go install` and prebuilt GitHub Release binaries.
- **Acceptance tests** — flagship lifecycle demo, trap-repo retrieval benchmark,
  two-clone concurrent-sync convergence, and hook latency budget.

[0.3.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.3.0
[0.2.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.2.0
[0.1.1]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.1.1
[0.1.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.1.0
