[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html

# Changelog

All notable changes to TeamMemory are documented here. The format is based on
[Keep a Changelog], and this project adheres to [Semantic Versioning].

## [0.7.0] - 2026-06-23

The first Phase 3 trust-and-safety release. It closes the cheap leak-prevention
gap before records enter the append-only Git ledger, and adds an opt-in
identity-aware confirmation mode so teams can require confirmations from a
different Git identity, not merely a different session.

### Added

- **Propose-time secret/PII scanning.** `tm propose` and `tm_propose` now scan
  memory titles, summaries, guidance, and evidence refs before appending the
  record. Secrets block by default; conservative PII findings warn by default.
  The behavior is policy-controlled via `propose_safety.secret_action` and
  `propose_safety.pii_action`.
- **Identity-aware independent confirmation.** Policies may now set
  `activation.independence: different_actor`. CLI and MCP-created agent records
  stamp `actor.email` from `git config user.email`; when both memory and
  observation have email, confirmations only count as independent if the email
  values differ. Records without email degrade to the existing session-based
  independence rule for compatibility with old ledgers, CI bots, and unset Git
  identities.

### Changed

- `prd.md` marks both Phase 3 trust-and-safety items as shipped, and `README.md`
  documents the new `different_actor` policy option.

## [0.6.3] - 2026-06-22

Bugfix release hardening hook startup and Codex nudge delivery after dogfooding
with fetched team-memory branches and Codex Stop hooks.

### Fixed

- **Already-fetched ledger branches no longer break hook startup.** When a repo
  has exactly one remote-tracking `teammemory` branch, `tm init`, `tm doctor`,
  and hook-time environment opening now adopt it into the local
  `refs/heads/teammemory` ref instead of treating the ledger as missing. If
  multiple remote-tracking candidates exist, tm still refuses to guess.
- **Codex Stop-hook nudges no longer emit invalid JSON.** Codex Stop advisories
  are now queued silently and drained through the next `UserPromptSubmit`
  `additionalContext` channel, while requirement blocks still use Codex's
  supported top-level `decision` / `reason` shape.

## [0.6.2] - 2026-06-17

Bugfix release closing the third Claude Code surfacing gap (the MCP
session-id template-literal case), plus a new five-act hero GIF in the
README that demonstrates the full validation flywheel end to end.

### Fixed

- **MCP boundary swallowed `${CLAUDE_SESSION_ID}` as a literal value.**
  The v0.6.1 envSession fallback only covered the Bash-subprocess case;
  the MCP server's `tm_propose` / `tm_observe` handlers still stored
  `args.Session` verbatim. The MCP schema docs said *"Use
  $CLAUDE_SESSION_ID"*, which agents reading the schema sometimes
  interpreted as "put this literal string in the field" — both the
  proposed memory and the confirming observation then carried the
  template string as their session id, the independence check
  collapsed to a string-equality false positive, and the memory never
  auto-activated. A new `internal/sessionid` package now centralizes
  the resolution chain (explicit non-template → `$CLAUDE_SESSION_ID`
  → `.git/tm/current-session.txt`), recognizes `${VAR}` / `$VAR` /
  `<placeholder>` shapes as unset, and is called from both the CLI
  flag defaults and the MCP handlers. The schema docs now tell agents
  to leave `session` empty.

### Changed

- **README hero GIF** — replaced the original single-block recording
  with a five-act flywheel demonstration: Agent A proposes, Agent B
  independently confirms (memory auto-activates to warning), Agent C
  hits the warning and complies by adding the missing `.down.sql`,
  the maintainer promotes to requirement, Agent D tries the same
  edit and reports the block by name. All four Claude sessions are
  real — the GIF is reproducible via `bash demo/hero-setup.sh` +
  `vhs demo/hero.tape`. 3:10, 2.4MB.

## [0.6.1] - 2026-06-17

Bugfix release closing two Claude Code surfacing gaps discovered while
dogfooding v0.6.0, plus a UX fix to `tm search`.

### Fixed

- **Claude Code Stop-hook stdout doesn't reach the agent.** A live session
  that fired three `self_review` nudges showed zero `tm:`-prefixed text in
  the transcript — the v0.6.0 plain-stdout-on-Stop path doesn't actually
  surface on Claude Code 2.1.x. `tm nudge --hook` now also queues each
  Claude nudge into `journal.Pending`, and `tm signal --hook --prompt`
  (UserPromptSubmit) drains the queue via `hookSpecificOutput.additionalContext`
  — the channel verified to surface. The Render path still emits plain
  stdout for harnesses where that works. Pinned by
  `TestNudgeHookQueuesPendingOnClaude` and
  `TestPromptSignalDrainsPendingViaAdditionalContext`.
- **`$CLAUDE_SESSION_ID` isn't exported into the Bash tool's subprocess** on
  Claude Code 2.1.x, so every shelled-out `tm ack` / `tm propose` /
  `tm observe` silently fell back to TTL session attribution. Hook payloads
  still carry `session_id`, so `tm signal --hook` and `tm nudge --hook` now
  write `.git/tm/current-session.txt`, and `envSession()` reads it after the
  env var. Discovery order is documented in `prd.md §10.2`. Pinned by
  `parse_test.go`.
- **`tm search` / `tm_search` now distinguish "no searchable tokens" from
  "no matches".** Queries like `*`, `**`, or any all-punctuation input that
  tokenized to nothing previously printed the same `No results.` line as a
  genuine miss — callers had no way to tell an empty ledger from a
  meaningless query. Both call sites now emit a distinct message and steer
  the caller to a real keyword or to `tm status` / `tm_status` for ledger
  contents.

### Changed

- **OSS hygiene files** — `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, and GitHub issue templates added for first-impression
  parity with mature OSS projects.
- **README** — restructured for OSS first-impression (install up top,
  trimmed mid-sections, roadmap moved down); added a measured **Context
  cost** section with token figures.

## [0.6.0] - 2026-06-17

The "package-manager distribution" release — Phase 2 closes (`prd.md §17`).
`tm` is now installable via the standard OSS channels alongside `go install`
and the GitHub Releases archives.

### Added

- **Homebrew tap** — `brew install AndreasSteinerPF/tm/tm`. GoReleaser commits
  the formula to [`AndreasSteinerPF/homebrew-tm`](https://github.com/AndreasSteinerPF/homebrew-tm)
  on each release.
- **Scoop bucket** — `scoop bucket add tm https://github.com/AndreasSteinerPF/tm-scoop && scoop install tm`.
  GoReleaser commits the manifest to [`AndreasSteinerPF/tm-scoop`](https://github.com/AndreasSteinerPF/tm-scoop)
  on each release.
- **POSIX install script** — `curl -fsSL https://raw.githubusercontent.com/AndreasSteinerPF/team-memory/main/install.sh | sh`.
  Detects OS/arch, fetches the matching archive from the latest release,
  verifies its SHA-256 against `checksums.txt`, and drops `tm` into
  `$HOME/.local/bin` (overridable via `TM_INSTALL_DIR`). No deps beyond
  `curl` and `tar`.

## [0.5.0] - 2026-06-17

The "polished separate-remote UX" release. The separate-remote escape hatch
for teams under branch protection (`prd.md §7.1`) is now a first-class
surface: a `tm remote` subcommand, validation + push on `tm init`, and a
push-failure capture pipeline that surfaces stable rejections through
`tm sync`, `tm status`, and `tm doctor` instead of silently swallowing them.

### Added

- **`tm remote` subcommand** — `tm remote show` prints the current ledger
  remote and its source (configured vs. default), resolving bare names to
  URLs via `git remote get-url`. `tm remote set <name-or-url>` validates the
  target with `git ls-remote` (5s timeout) before writing `git config
  tm.remote`; `--force` skips validation. `tm remote unset` is idempotent.
  Bare `tm remote` aliases to `show`. (`prd.md §7.1`, `§10.5`)
- **`tm init` validates and seeds the remote.** When `--remote <url>` is
  given (or `origin` is the implicit fallback), `tm init` runs `ls-remote`
  to confirm reachability, then attempts a best-effort push of the orphan
  `teammemory` ref so teammates can fetch it immediately. A reachable URL is
  stored as `tm.remote`; an unreachable URL is **not** stored, and the user
  sees a one-line `Fix: ... tm remote set ...` hint. `--no-push` skips both
  steps for offline / CI bootstrap. Init never errors out on remote
  failures — the local ledger is always usable. (`prd.md §7.1`)
- **Push-failure capture and classification.** Every push attempt — from
  `tm sync`, the background pushes after `tm propose` / `tm observe`, and
  `tm init`'s seed — classifies its stderr into one of four kinds
  (`protected_branch | auth | network | unknown`) and writes the latest
  outcome to `.git/tm/push_failure.json` (local-only, same convention as
  `.git/tm/acks` and `.git/tm/nudge`). Consecutive same-kind failures
  increment a counter; any successful push clears the record. (`prd.md §7.1`)
- **Diagnosis on every surface.**
  - `tm sync` (foreground) prints the classified kind plus a kind-specific
    fix hint on stderr when its push fails.
  - `tm status` appends a `⚠ Last N background pushes to "<remote>"
    rejected (<kind>). Fix: ...` line when consecutive ≥ 2 and the record
    is fresh (within 7 days). A single transient failure is silent.
  - `tm doctor` adds a `Recent push failures` check — `sevOK / "none"` when
    clear, `sevWarn` with the diagnosis when fresh.
  - On `protected_branch`, the fix hint names `tm remote set` as the
    recovery path — closing the loop from detection to action.
- **`ledger.OnPushResult` callback hook.** `internal/ledger.Ledger` gained an
  `OnPushResult(remote, stderr, err)` field invoked after every push attempt
  in `doPush`, with best-effort stderr extraction from the wrapped error
  message. The CLI's `openEnv` installs the callback so every `Sync`
  (foreground or background) records its outcome — no per-call-site wiring.
- **`git.ValidateRemote(repoDir, remote, timeout)`** — shared helper for
  `tm init` and `tm remote set`. Runs `git ls-remote` under a
  `context.WithTimeout`; returns verbatim stderr on failure so callers can
  show the user exactly what git said.

### Changed

- **README — separate-remote mode is documented as a recipe**, not just a
  config-key footnote. New `### Branch protection / separate-remote mode`
  subsection under `## Sync`. The `tm init --remote` paragraph now mentions
  validation, the seed push, and `--no-push`. The Commands table lists
  `tm remote`.
- **`prd.md`** — §7.1 cross-references the new subcommand; §10.5 bumped to
  eighteen commands; §15 "Branch protection" risk row now points at
  `tm remote set` and the new diagnosis surfaces instead of a generic
  "documented setup note"; §17 Phase 2 marks "Polished separate-remote UX"
  as **Shipped**.

### Fixed

- **First push from `tm init` no longer races propose's background push.**
  `tm init`'s seed uses a raw `git push refs/heads/teammemory:refs/heads/teammemory`
  rather than `ledger.Sync` — `Sync` would fetch-then-push and, on a remote
  that already had the orphan tip, pull state into the freshly created local
  ledger before the user's first `tm propose`. The downstream
  `TestSyncUnionMergeAcrossClonesCLI` e2e exercises the un-merged case and
  caught this. The `e2e/push_test.go::TestProposeTriggersBackgroundPush`
  poll was also tightened to wait for the actual memory record rather than
  branch existence, since init now seeds the branch before propose's
  background push lands.

## [0.4.0] - 2026-06-16

The "memory types and cross-memory linking" release. Adds the
`successful_pattern` memory type (Phase 2's deferred sixth) and two new
observation kinds — `mark_duplicate` and `supersede` — that link memories to
each other for the first time. Derived state gains a cross-memory layer
(`derive.Context`) so a substantiated supersession transitions one memory's
status based on another's observations.

### Added

- **`successful_pattern` memory type** — repeatedly-applied refactors or
  approaches with a measurable outcome. Low risk for ranking, but carries a
  **type-specific activation gate**: stays `provisional` until at least one
  independent session confirms it (or a maintainer approves it), regardless
  of the low-risk tier's normal auto-activation. The gate is the
  spam-control: a single function that worked once cannot unilaterally
  become an active pattern. (`prd.md §5.2`, `§8.2`)
- **`mark_duplicate` observation kind.** File it on the duplicate memory
  naming the kept memory in `--canonical-id`. Auto-effect — the duplicate
  immediately flips to status `duplicate` and is excluded from retrieval.
  A later `confirm`/`approve` on the duplicate resolves it (back to active).
  (`prd.md §5.3`, `§8.2`)
- **`supersede` observation kind.** File it on the *new canonical* naming
  the obsolete memory in `--supersedes`. Substantiated cross-memory:
  the obsolete memory transitions to status `superseded` only after either
  a human `approve` or an independent `confirm` lands on the new canonical
  (mirrors `adjust_scope`-broadening substantiation). Pending claims are
  visible in `tm show <obsolete>` and `tm list --pending-supersede`. The
  reason text names the new canonical. (`prd.md §5.3`, `§8.5`)
- **Two new statuses with proper precedence.** Status ladder is now:
  `rejected` > `stale` > `duplicate` > `superseded` > `contested` >
  `successful_pattern` gate > `active` > `provisional`. Excluded from
  retrieval entirely: `rejected`, `stale`, `duplicate`, `superseded`.
  (`prd.md §8.2`, `§11.4`)
- **Cross-memory derive primitive** (`internal/derive/Context`,
  `BuildContext`). The first place per-memory derivation sees facts from
  other memories' observations. Computed once per ledger pass; the index
  incremental `Update` fans out canonical-status changes to dependent
  memories so the materialized table stays consistent. Fast-path skips the
  full memories load when no cross-memory observations exist, so hook
  latency is unchanged on the common case. (`prd.md §8.5`)
- **Multi-hop cycle detection** at observe time. `mark_duplicate` and
  `supersede` walk the existing canonical/supersedes chain at file time
  and warn (don't block) when a new observation would close a cycle of
  any length (A→B→A, A→B→C→A, …). Operator may be deliberately
  consolidating; the warning ensures they know they're about to hide every
  memory in the chain from default retrieval. (`prd.md §8.5`)
- **Orphan revival — canonical-status changes propagate.** If canonical A
  becomes non-active (`reject` or unresolved `mark_stale`), any memory B
  that pointed at A via `mark_duplicate` or `supersede` reverts to its
  un-orphaned status. Reversible: a later `confirm` resolving a
  `mark_stale` revives A *and* re-marks the dependents; `reject` is
  terminal so its revival is permanent. (`prd.md §8.5`)
- **New CLI filters and surfaces.** `tm list --duplicate`,
  `--superseded`, and `--pending-supersede` filter by the new statuses
  and the pending cross-memory claims. `tm show <obsolete>` lists pending
  supersession claims naming the memory. `tm status` reports counts for
  the new statuses plus a separate `Pending supersede claims: N` line.
  (`prd.md §10.5`)
- **Retrieval cap is now additive: 5 active + 2 provisional.** Previously
  the cap was 5 total with provisional inside the same budget — which
  starved provisional surfacing in well-instrumented areas (exactly where
  new proposals are most likely). The two budgets are now separate;
  `duplicate`/`superseded` rows are excluded entirely. (`prd.md §11.3`,
  `§11.4`)

### Changed

- **Shared agent-facing guidance constants.** A single
  `MemoryWorthyGuidance` (full enumeration with the new
  `successful_pattern` non-example) and `MemoryWorthyShortForm` (one-sentence
  form) in `internal/model/guidance.go` back the MCP `tm_propose`
  description, the SessionStart briefing, and `tm export` instruction
  preambles — so the same canonical text appears on every surface and
  cannot drift. (`prd.md §10.3`, `§10.1`, `§10.6`)
- **`tm_propose` accepts `successful_pattern`** in both the CLI validator
  and the MCP tool. The MCP `Type` field schema lists all six types; the
  tool description carries the activation-gate caveat so agents aren't
  surprised by the initial `provisional` status.
- **`tm_observe` accepts `mark_duplicate` and `supersede`** in both
  surfaces. Cross-memory validation rejects self-references and missing
  IDs, and **warns (not blocks)** when the referenced memory is in a
  non-active state — same target-warning policy applied to the
  observation's own target. CLI surfaces the warning on stderr; MCP
  appends it to the tool result text. (`prd.md §10.3`, `§10.5`)
- **Index schema bumped to v4.** Existing v3 indexes auto-rebuild on the
  next `tm` invocation. No ledger migration needed — the new YAML
  observation fields (`canonical_id`, `supersedes`) are `omitempty`, so
  pre-existing observation files load unchanged.

### Fixed

- **Policy-driven independence in `adjust_scope` substantiation.** The
  broadening-substantiation rule previously hardcoded the
  `"different_session"` independence mode; it now respects
  `policy.activation.independence` (same as supersede substantiation), so
  the stricter `"different_session_and_branch"` mode applies consistently
  across cross-memory checks. (`prd.md §8.5`)
- **`tm observe` warn helpers use an O(1) status lookup.** The
  `warnIfNonActive` helper introduced for cross-memory validation
  initially scanned the full index per call; it now uses a new
  single-row `idx.Status(id)` method.

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

[0.7.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.7.0
[0.6.3]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.6.3
[0.6.2]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.6.2
[0.6.1]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.6.1
[0.6.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.6.0
[0.5.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.5.0
[0.4.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.4.0
[0.3.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.3.0
[0.2.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.2.0
[0.1.1]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.1.1
[0.1.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.1.0
