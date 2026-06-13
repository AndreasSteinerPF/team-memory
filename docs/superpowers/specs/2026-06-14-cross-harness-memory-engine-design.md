# Cross-harness memory engine: proposing nudges + deterministic injection everywhere

**Status:** design (approved for planning)
**Date:** 2026-06-14
**Touches:** `prd.md` §10 (hooks), §149 (deterministic vs voluntary), §537 (anti-spam / "agents ignore the tool"), §584 (hook-first integration)

---

## 1. Problem & goals

TeamMemory's loop has two halves with very different reliability:

- **Consumption** is deterministic *on Claude Code only*. The PreToolUse hook injects relevant
  memories at edit time (§584) — the agent never has to choose to look. On every other harness,
  consumption is voluntary (`tm_check_action` via MCP) plus the SessionStart brief.
- **Production** (`tm_propose`) and **validation** (`tm_observe`) are voluntary *everywhere*, including
  Claude Code. They fire only if the model notices a memory-worthy moment and chooses to break task
  flow to record it. A tool description is weakly salient — it influences behavior only once the model
  is already enumerating tools. The SessionStart brief states the rule once, where it decays in salience
  long before the memory-worthy moment arrives.

Two gaps follow, and this design closes both:

1. **Proposing/observing under-fires.** We have no near-moment trigger; we rely on the model
   remembering a rule delivered at session start.
2. **Deterministic injection is Claude-Code-only.** As of 2026 every major harness (Codex, Copilot CLI,
   Cursor, Gemini CLI) has shipped a hook system. The "degraded everywhere but Claude Code" stance in
   §537 is obsolete.

**Goals.**

- A **near-moment nudge engine** that escalates the highest-value moments to pointed `tm_propose` /
  `tm_observe` prompts, while keeping the verbs voluntary and avoiding manufactured-junk proposals.
- **Deterministic retrieval on all five harnesses**, factored into the parts each harness can actually
  support: requirement **blocking** pre-edit, advisory **injection** at edit time.
- A **single shared engine** in the `tm` Go binary with thin per-harness adapters, so a new harness is
  *config + one adapter file*, not a reimplementation.

**Non-goals.** Detecting natural-language user corrections heuristically (that is the periodic
self-review's job). Cross-session signal correlation. Changing the trust/derivation model.

---

## 2. Architecture

The engine lives in the `tm` Go binary behind `--hook` subcommands. Harnesses differ only in event
names, payload schema, config format, and packaging — never in the core logic.

```
harness event (JSON on stdin)
        │
        ▼
 per-harness adapter  ──►  internal signal model  ──►  shared engine  ──►  JSON on stdout
 (schema translation)                                   (journal, policy,    (inject / block /
                                                          retrieval, nudge)    nudge / silent)
```

Three engine entry points, wired to harness events by the adapter:

- **`tm check-action --hook`** (pre-tool event): requirement **block** decision; on Claude Code only,
  also advisory **inject** (allow + context). See §5.
- **`tm signal --hook`** (post-tool event): records signals to the session journal **and** emits
  advisory injection for the edited path (§5). Never blocks, never interrupts mid-turn.
- **`tm nudge --hook`** (turn-end event): applies the nudge policy and emits at most one nudge (§4).

Plus the existing **`tm brief`** (session-start event / instruction file) for the standing
"when to remember" instructions.

**Session journal.** `.git/tm/nudge/<session-id>.json` — local-only, never a ledger record, keyed by
the harness's session id, TTL-expired exactly like acks (§388). Holds per-path edit counts (with the
turn index of each edit), last failing/passing command signatures, revert events, memories surfaced for
this session, prompt-submit turn markers, nudges already fired, advisory injections already delivered,
and a turn counter. **Surfaced memories are written by whichever hook does the surfacing** —
`tm signal --hook` (post-tool advisory inject, all harnesses) and `tm check-action --hook` (pre-edit
inject on Claude Code) — so the surfaced-but-unobserved signal (§3) has a reliable source. Prompt-submit
markers plus the per-edit turn index are what let the user-intervened signal (§3 Tier B) detect "edit P →
user spoke → edit P again."

**Adapter contract.** Each adapter is one small file implementing: `parse(event JSON) → internal event`
(tool name, tool input, result/exit-or-failure flag, session id, paths) and `render(decision) → harness
JSON` (allow / deny+reason / inject context / nudge text). Everything downstream is shared.

---

## 3. Signal catalog & two-tier model

A signal earns inclusion only if it is **deterministically detectable**, **maps to a memory type or
sharpens the self-review**, and names a moment **the agent plausibly wouldn't act on unprompted**. The
periodic self-review is the long-tail backstop, so the heuristic set need not be exhaustive — it exists
to (a) upgrade the highest-frequency/value moments to *pointed* nudges and (b) aim the self-review.

**Tier A — self-classifying** (carry enough to suggest a concrete proposal):

| Signal | Detection | Maps to |
|---|---|---|
| **fail→pass recovery** | A command with signature S fails, **an Edit/Write happens**, then a command with signature S succeeds. Only the fail→pass *boolean transition* matters — never the numeric code. | `failed_attempt` propose |
| **revert / reset** | Bash containing `git revert`, `git reset --hard`, `git checkout -- <path>`, `git restore`. | `failed_attempt` / `fragile_area` propose |
| **edit churn** | Edit/Write/MultiEdit count on one path crosses `churn_threshold` (default 3) in a session. | `fragile_area` propose |
| **surfaced-but-unobserved** | A memory surfaced for path P; the session edited P and ended with no `observe` on it by this session. | `observe` (confirm/contradict) |
| **drift-anchor edited** | The agent edits a file flagged with a drift annotation (anchor changed since the memory was recorded). | `observe` (`mark_stale` / `adjust_scope`) |

`signature(S)` = normalized head of argv (binary + subcommand). The intervening edit separates a *lesson*
("changing the migration made it pass") from a *transient* ("re-ran after a network blip / fixed a
command typo"). The candidate lesson is attributed to the files edited between the two runs.

**Tier B — attention-flag** (flag the moment; the model supplies the content via the self-review):

| Signal | Detection | Behaviour |
|---|---|---|
| **user intervened mid-file** | The agent edited path P, a user prompt landed (prompt-submit event), then P was edited again. | Sharpens the periodic self-review with a pointed question ("the user redirected you while editing `auth/` — a constraint to record?"). Does **not** pre-fill a type. |

> **Why a proxy, not the abort itself.** The cleaner signal — "user *aborted/denied* the edit, then the
> agent retried differently" — is **not hook-observable on any harness**: no hook fires on a manual
> permission denial or on an ESC interrupt (verified against Claude Code, Codex, Copilot, Cursor, Gemini
> docs, 2026-06-14). The prompt-submit-bracketed re-edit above is the achievable proxy. Do not replace it
> with abort detection without re-verifying that a denial/interrupt hook has since shipped.

**Deliberate non-signals:** a command that only ever fails (no recovery — a broken build, not a lesson
yet); a path already covered by a memory the agent didn't contradict; raw NL-correction classification
(self-review's job); cross-session search patterns.

Detection lives in a `signal` package with one pure function per signal type,
`(event, journalState) → []Signal`, each table-tested against synthetic event sequences.

---

## 4. Nudge & anti-spam policy

The guiding fear is the inverse of under-proposing: a nudge that **manufactures junk** because the model
proposes to satisfy the prompt. The policy is built to prevent that.

`tm nudge --hook` runs at every turn-end:

1. Load the journal; collect signals recorded since the last nudge.
2. **Suppress-if-acted** (the key anti-nag rule): for each signal, query the local index for a
   `propose`/`observe` authored by *this* session covering the signal's path/memory since the signal
   timestamp. If the agent already acted, mark resolved and drop it. Never nag about what's done.
3. If a **Tier A** signal survives → one pointed nudge for the highest-priority one.
4. Else if a **Tier B** signal survives → one aimed self-review.
5. Else if the periodic cadence elapsed (no nudge in the last `self_review_every` turns *and* the session
   has ≥1 edit) → one generic self-review.
6. Else stay silent.

**Priority:** `observe` signals (surfaced-but-unobserved, drift-anchor) rank **above** propose signals —
observes unblock provisional→active (the thing that decays if neglected) and are more specific. Then
fail→pass > revert > churn.

**Anti-spam budget.**
- `nudge.max_per_session` (default **3**) — hard ceiling on total nudges.
- `nudge.cooldown_turns` (default **3**) between nudges, except a `requirement`-related observe may bypass.
- Per-signal dedup keyed by `(type, path)`.
- The periodic self-review is bounded by the same ceiling, so it can't become turn-by-turn noise.

**Wording is deliberately low-pressure**, one line, names path + suggested verb, and licenses dismissal:

```
tm: recovered from a failing `go test` after edits in internal/index/ —
    if that fix encodes a non-obvious lesson, tm_propose a failed_attempt; otherwise ignore.
```

The "otherwise ignore" plus `tm_propose`'s own quality gate and the propose-time duplicate warning (§539)
mean the nudge surfaces the option without pressuring compliance.

---

## 5. Deterministic retrieval: PreToolUse-block + PostToolUse-inject

Deterministic retrieval has two jobs (§378), and verified hook capabilities split them across two events.
**Crucially: a pre-tool hook can *block* on every harness, but can *inject advisory context while
allowing* only on Claude Code.** (Codex `additionalContext` is rejected by the CLI — issue #19385;
Copilot's CLI drops it — issue #2585; Cursor delivers hook text only on deny; Gemini `BeforeTool` exposes
only `tool_input`, and `systemMessage` is user-only.)

So:

- **Requirement enforcement → pre-tool event (block).** An unacknowledged `requirement` memory matching
  the edit → deny with guidance + required checks; the agent runs the checks, `tm ack`s, retries (§378,
  §388). This is the half that *must* be pre-edit, and the deny path is universal. **All five harnesses.**
- **Advisory surfacing → post-tool event (inject).** `warning`/active memories for the touched path →
  injected as the harness's post-tool context field (`additionalContext` / `additional_context`). Post-edit
  rather than pre-edit, but it informs the next edit in a multi-step task and beats today's voluntary-only
  baseline. **All five harnesses.** Injection is scoped to memories matching the edited path, deduped per
  session against the journal, and capped at `inject.advisory_max_per_session` (§7) — reusing the same
  journal/dedup machinery as the nudge engine, with its own separate budget.
- **Claude Code keeps the superior path:** advisory injected *pre*-edit via PreToolUse allow+inject. This
  is the one fidelity difference; documented honestly.

The advisory injector and the signal recorder are the **same post-tool entry point** (`tm signal --hook`):
one hook call both injects relevant memories and records signals.

---

## 6. Per-harness implementations

All five wrap the same engine. Each section: event mapping, packaging, fail→pass sensor, and a
verification checklist for the unknowns we will **not** trust the docs on.

**Value tiers & build order (highest-value path per harness).** The value ranking is identical
everywhere — *deterministic retrieval is the headline (§584); the nudge engine is the second increment;
MCP verbs + the brief are the floor* — so "highest-value path" is really about build order:

- **Tier 0 — floor, ship to all five immediately:** MCP voluntary verbs + the instruction-file brief.
  Zero hook work; strictly better than nothing on every harness.
- **Tier 1 — the headline, all five:** requirement **block** (pre-tool) + advisory **inject** (post-tool,
  pre-edit on CC). This is the deterministic-retrieval win and where most user value lands.
- **Tier 2 — the nudge engine:** signals + policy + self-review.

Per-harness sequencing within that:
- **Claude Code** — reference implementation; build the full engine here first.
- **Codex, Copilot CLI** — near-drop-in ports: event taxonomy mirrors Claude Code and both have plugin
  systems that bundle hooks + MCP + instructions in one artifact. Do these right after Claude Code.
- **Cursor, Gemini CLI** — full engine still ports, but Tier 0 + Tier 1 land trivially first; the nudge's
  fail→pass uses the **failure-flag** variant (no exit code) and packaging is native (Cursor `hooks.json`
  / Gemini extension). Ship floor + retrieval, then add the nudge with the failure-flag sensor.

### 6.1 Claude Code (reference implementation)

| Logical | Event | Notes |
|---|---|---|
| brief | SessionStart | existing `tm brief` |
| block + advisory inject (pre-edit) | PreToolUse | allow+inject supported here |
| signal record + advisory inject (post-edit) | PostToolUse | exit code present |
| user-intervened | UserPromptSubmit | |
| nudge | Stop | fires per turn-end |

- **fail→pass sensor:** exit code (clean).
- **Packaging:** Claude Code plugin (PreToolUse + PostToolUse + UserPromptSubmit + Stop + SessionStart + MCP).

### 6.2 Codex CLI

| Logical | Event |
|---|---|
| brief | SessionStart |
| requirement block | PreToolUse (`permissionDecision: deny` / `permissionDecisionReason`) |
| signal + advisory inject | PostToolUse (`hookSpecificOutput.additionalContext`) |
| user-intervened | UserPromptSubmit |
| nudge | Stop |

- **fail→pass sensor:** `tool_response.exit_code` (third-party-confirmed) → **verify**; fall back to
  output-failure inference if absent.
- **Packaging:** `.codex-plugin/plugin.json` bundling MCP (`.mcp.json`), `hooks/hooks.json`, and `AGENTS.md`.
- **Verify before depending:**
  - [ ] PreToolUse/PostToolUse **fire for `apply_patch` (file edits)**, not just `Bash` — historically Bash-only (open issues). If still Bash-only, edit-time retrieval covers commands but not file writes.
  - [ ] `tool_response.exit_code` field exists on the installed version.
  - [ ] PostToolUse `additionalContext` is honored (the *pre*-tool bug #19385 does not apply to post-tool).

### 6.3 Copilot CLI

| Logical | Event |
|---|---|
| brief | sessionStart + `AGENTS.md`/`copilot-instructions.md` |
| requirement block | preToolUse (`permissionDecision`) |
| signal + advisory inject | postToolUse (`additionalContext`, appended to `textResultForLlm`, ≤10 KB) |
| user-intervened | userPromptSubmitted |
| nudge | agentStop |

- **fail→pass sensor:** SDK `toolResult.exitCode` for the `shell` tool; for script hooks use the
  `postToolUseFailure` event as the fail marker.
- **Packaging:** Copilot plugin bundling `hooks.json` + `mcp-config.json` + `AGENTS.md`.
- **Verify:**
  - [ ] post-tool `additionalContext` flows through a **script/command** hook (documented for SDK; verify for script — else ship the SDK hook).
  - [ ] script post-tool hook receives the shell `exitCode` (else rely on `postToolUseFailure`).

### 6.4 Cursor

| Logical | Event |
|---|---|
| brief | Project Rule with `alwaysApply: true` (or `AGENTS.md`) |
| requirement block | preToolUse / beforeShellExecution (`permission: "deny"` or exit 2) |
| signal + advisory inject | postToolUse / afterShellExecution (`additional_context`) |
| user-intervened | beforeSubmitPrompt |
| nudge | stop |

- **fail→pass sensor:** **failure flag** — `postToolUseFailure` marks the fail; a later same-signature
  `afterShellExecution` with no failure marks the pass. (No numeric exit code is exposed; we don't need it.)
- **Packaging:** `.cursor/hooks.json` + `.cursor/mcp.json` + a `.cursor/rules/*.mdc` brief.
- **Verify:**
  - [ ] post-tool `additional_context` (snake_case) injects model-visible text on allow.
  - [ ] `afterShellExecution` / `postToolUseFailure` payloads carry enough to compute the command signature.

### 6.5 Gemini CLI

| Logical | Event |
|---|---|
| brief | SessionStart + `GEMINI.md` |
| requirement block | BeforeTool (`decision: "deny"` / `reason`) |
| signal + advisory inject | AfterTool (`hookSpecificOutput.additionalContext`) |
| user-intervened | BeforeAgent |
| nudge | AfterAgent |

- **fail→pass sensor:** **failure flag** — `AfterTool` `tool_response.error` set = fail; empty on a later
  same-signature run = pass.
- **Packaging:** a Gemini **extension** bundling MCP server + hooks + `GEMINI.md` (extensions can bundle
  hooks as of v0.26).
- **Verify:**
  - [ ] confirm against the pinned release tag, not `main` (schema may drift).
  - [ ] `AfterTool.additionalContext` is model-visible (it is per reference; `systemMessage` is user-only).

---

## 7. Config & cross-harness matrix

New `nudge.*` table alongside `sync.*`, all overridable; `TM_NUDGE=off` env overrides per session.

| Key | Default | Purpose |
|---|---|---|
| `nudge.enabled` | `true` | master switch |
| `nudge.max_per_session` | `3` | ceiling on nudges per session |
| `nudge.cooldown_turns` | `3` | min turns between nudges |
| `nudge.self_review_every` | `8` | turns of silence before a periodic self-review |
| `nudge.churn_threshold` | `3` | edits to one path before a churn signal |
| `nudge.signals` | all on | per-signal toggles |
| `inject.advisory_max_per_session` | `5` | cap on post-tool advisory injections |

**Capability matrix (verified 2026-06-14):**

| | MCP verbs | brief | requirement block (pre) | advisory inject | inject timing | nudge | fail→pass sensor |
|---|---|---|---|---|---|---|---|
| Claude Code | ✅ | ✅ | ✅ | ✅ | **pre-edit** | ✅ | exit code |
| Codex | ✅ | ✅ | ✅ | ✅ | post-edit | ✅ | exit code* |
| Copilot CLI | ✅ | ✅ | ✅ | ✅ | post-edit | ✅ | exit code / failure flag |
| Cursor | ✅ | ✅ | ✅ | ✅ | post-edit | ✅ | failure flag |
| Gemini CLI | ✅ | ✅ | ✅ | ✅ | post-edit | ✅ | failure flag |

\* verify the field on the installed version.

---

## 8. Testing

- **Signal detection:** table-driven tests feeding synthetic event sequences (failing `go test` → Edit →
  passing `go test`; revert; churn threshold; drift-anchor edit) into each pure `signal` function.
- **Policy:** golden tests over journal fixtures — suppress-if-acted, dedup, cooldown, max-per-session
  ceiling, priority ordering (observe before propose), periodic-cadence gating.
- **Suppression loop:** a `propose` authored by the session for the signal's path silences the nudge.
- **Adapters:** per-harness fixtures — a captured (or doc-derived) event JSON for each harness → assert the
  adapter produces the right internal event and renders the right decision JSON. This is where the
  fail→pass sensor difference (exit code vs failure flag) is exercised per harness.
- **Latency:** `tm signal --hook` and `tm nudge --hook` each under the 100ms budget on a 1,000-memory
  ledger, no network (§375, §545).
- **e2e:** a `nudge.txtar` script (matching `propose.txtar` / `checkaction.txtar`) driving a scripted
  session: signals recorded → nudge emitted → agent proposes → suppression on the next turn.

---

## 9. prd.md deltas (same commit as implementation)

- **§10 (hooks):** add the `signal`/`nudge` hook verbs and the per-harness event mapping; document the
  PreToolUse-block / PostToolUse-inject split.
- **§149:** add the near-moment nudge as a third delivery mechanism between deterministic delivery and
  voluntary recall.
- **§537:** reframe "agents ignore the tool" — mitigation is now *session brief + near-moment nudge
  engine*, and deterministic injection is *universal*, not Claude-Code-only.
- **§584:** reframe "hook-first integration" from Claude-Code-specific to a shared engine with per-harness
  adapters; record the one fidelity difference (advisory pre-edit on CC, post-edit elsewhere).
- Add the `nudge.*` / `inject.*` config to the config section.

---

## 10. Open verification items (carried into the plan)

These are flagged inline above; collected here so the plan tracks them as explicit steps. We confirm each
against a **live payload on the installed version**, not the docs:

1. Codex: do pre/post-tool hooks fire for `apply_patch`? Does `tool_response.exit_code` exist?
2. Copilot: does a script (non-SDK) post-tool hook receive `additionalContext` and the shell `exitCode`?
3. Cursor: does `additional_context` inject on allow? Signature data on `afterShellExecution` / `postToolUseFailure`?
4. Gemini: confirm schema against the pinned release tag; `AfterTool.additionalContext` model-visibility.
5. All: post-tool `additionalContext` actually reaches the model (the pre-tool bugs do not apply here).
