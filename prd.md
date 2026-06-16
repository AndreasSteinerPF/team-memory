# PRD: TeamMemory

## 1. Product Summary

**Product name:** TeamMemory
**Category:** Open-source developer tool
**Tagline:** Agents propose. Agents observe. Teams remember.
**One-liner:** TeamMemory is a Git-backed collaborative memory ledger for coding agents. Agents propose repo-scoped memories during normal work; other agents confirm, contradict, or refine them when they encounter related code; validated memories reach future agents deterministically through an edit-time hook, not just a voluntary tool call.

TeamMemory is not a general memory system, not an agent framework, and not a documentation platform. It is a focused system for preserving **future-action-relevant project judgment**: failed attempts, constraints, fragile areas, stale docs, and undocumented decisions that should influence future coding-agent behavior.

## 2. Product Thesis

Coding agents repeatedly lose team-level context: prior failed approaches, hidden constraints, risky files, stale docs, undocumented decisions. Static context files (`CLAUDE.md`, `AGENTS.md`, IDE rules) help but do not evolve through work, and auto-capture memory tools accumulate without validation — which is exactly how memory poisoning happens.

The core thesis:

> Team memory should evolve through normal agent work. Agents propose memories when they discover reusable project judgment. Future agents encounter provisional memories, validate or contradict them with evidence during their own work, and gradually strengthen or weaken the team's shared memory. Validated memory reaches agents deterministically at edit time.

### 2.1 Positioning Against the Field

TeamMemory competes in a crowded 2026 memory landscape and must differentiate sharply:

| Competitor class | Examples | What they do | TeamMemory's difference |
|---|---|---|---|
| Auto-capture memory | claude-mem, Mem0/OpenMemory, agentmemory, Cipher | Observe sessions, compress, accumulate | Evidence-validated lifecycle: memories earn trust through independent confirmation; contradictions weaken them |
| Hosted team memory | Cloudflare Agent Memory, Supermemory | Shared memory profiles via hosted API | Git-native and local-first: the ledger lives in your repo, auditable via `git log`, no SaaS dependency |
| Platform-native memory | Claude Managed Agents memory/Dreaming, Cursor memories | Per-platform memory stores | Cross-agent and team-scoped: one ledger serves Claude Code, Codex, Cursor, Continue |
| Static context files | `CLAUDE.md`, `AGENTS.md`, `.cursor/rules` | Hand-maintained instructions | Evolves through work; context files become generated projections |

The wedge: **evidence-validated, git-native, governed team memory with a deterministic enforcement point.**

## 3. Goals

### 3.1 Product Goals

1. Provide a local-first, Git-backed memory ledger for coding agents, stored in the code repo itself without polluting code branches.
2. Allow agents to propose memories during normal development work.
3. Let other agents confirm, contradict, adjust, or stale-mark memories when they naturally encounter related work.
4. Deliver relevant memory deterministically at edit time via a Claude Code hook, and on demand via MCP and CLI.
5. Let memory evolve autonomously between agents — no human code review in the loop — while requiring human approval for high-impact enforcement.
6. Compute risk, status, confidence, and enforcement deterministically from policy, never from agent self-assessment.
7. Maintain a conflict-free, auditable Git history of memory evolution.
8. Avoid becoming a general-purpose personal memory system, generic RAG tool, or agent runtime.

### 3.2 Portfolio / Open-Source Goals

1. Demonstrate a serious applied-AI systems idea: agent memory as collaborative, governed, evolving project judgment.
2. Provide a runnable demo showing two agents on different branches validating the same provisional memory.
3. Attract interest from developers using Claude Code, Codex, Cursor, Continue, and other coding agents.
4. Establish a public technical artifact around agent reliability, memory governance, and action-time context.

## 4. Non-Goals

TeamMemory v1 will not be:

1. a full agent framework;
2. a chat assistant;
3. a hosted SaaS product;
4. a generic personal memory system;
5. a general RAG system over documentation;
6. a replacement for `CLAUDE.md`, `AGENTS.md`, or `.cursor/rules` (it generates them as projections);
7. a GitHub app or web dashboard;
8. a semantic knowledge graph;
9. a workflow/skills framework;
10. a system that lets agents create requirement-level rules without human approval;
11. a system that automatically records every interaction or file read.

## 5. Core Concepts

### 5.1 Memory

A memory is **durable project judgment that should influence future agent behavior**.

Examples:

* "Billing migrations require downgrade-path tests."
* "This API behavior is relied on by the Terraform provider."
* "This doc is stale; ADR-014 supersedes it."
* "Previous attempts to fix this rollback issue by editing generated migrations failed."
* "This path is fragile because changes often break release reconciliation."

Non-examples:

* "The project uses Python." (derivable from the repo)
* "This function validates invoices." (derivable from the code)
* "This test failed once because of a typo." (not durable)
* "This task is currently in progress." (session state)
* "Pass `--foo` on macOS but `--bar` on Linux." (system/OS-specific, not team judgment — would be wrong for part of the team)

### 5.2 Memory Types

Five types in v1. Typed envelope, free-form summary and guidance.

1. `failed_attempt` — an approach that was tried and failed, with evidence.
2. `constraint` — a rule on how work must be done here; `origin: team` (internal convention/policy) or `origin: external` (third-party compatibility, API contract).
3. `fragile_area` — a path where changes frequently break non-obvious things.
4. `stale_doc` — a document that is outdated or misleading, ideally with a pointer to what supersedes it.
5. `decision` — a decision that changes future work and is not written down anywhere else (including ownership/responsibility changes).

Deferred to roadmap: `ownership` and `successful_pattern` as dedicated types (ownership changes are expressible as `decision`; `successful_pattern` is the memory-spam magnet and needs the validation flywheel proven first).

### 5.3 Records: Memories and Observations

The ledger contains exactly two record kinds, both append-only, both immutable after creation:

* **Memory record** — the envelope: type, title, summary, guidance, scope, evidence, anchors, actor, timestamp.
* **Observation record** — a reaction to an existing memory: kind, summary, evidence, code context, actor, timestamp.

Observation kinds:

| Kind | Who | Meaning |
|---|---|---|
| `confirm` | agent or human | Independently hit the same issue / verified the memory, with evidence |
| `contradict` | agent or human | Found evidence the memory is wrong, with evidence |
| `adjust_scope` | agent or human | The lesson is right but the scope is wrong; carries `suggested_scope` |
| `mark_stale` | agent or human | The code or situation this memory describes no longer exists |
| `approve` | human only | Activate a critical memory, raise enforcement, or raise confidence |
| `reject` | human only | Kill a memory (terminal) |

Deferred to roadmap: `mark_duplicate`, `suggest_supersession` (need cross-memory linking), `broaden_scope` as a distinct kind (covered by `adjust_scope`), `add_evidence` (a `confirm` carries evidence), `record_usage` (voluntary telemetry agents won't reliably emit).

### 5.4 Derived State

Status, confidence, risk, enforcement, and effective scope are **never stored**. They are a deterministic pure function:

```text
state = f(memory, observations[], policy)
```

computed at read time and cached in a local SQLite index. Same records + same policy ⇒ same state on every machine. This eliminates merge conflicts (nothing shared is ever mutated), stored-state divergence, and agent self-assessment gaming. Section 8 specifies the function.

### 5.5 Provisional vs Active

* **Active** memory is served as trusted guidance by `check_action`.
* **Provisional** memory (not yet activated) is surfaced only when strongly related to the current action, capped, and framed as caution:

> "Possible lesson from prior work. Use as caution, not policy. Add a confirmation or contradiction if your work bears on it."

### 5.6 Activation vs Enforcement

Activation (is this memory trusted?) and enforcement (how strongly is it pushed?) are separate.

Enforcement levels: `hint` < `recommendation` < `warning` < `requirement`.

Most memories auto-activate at or below `warning` once they meet their risk tier's bar. Only a human `approve` can make a memory a `requirement`. In Claude Code, `requirement` memories block matching edits via the hook until acknowledged (Section 10.2).

## 6. Product Principles

1. **Deterministic delivery over voluntary recall.** The hook guarantees `check_action` happens at edit time; MCP covers the voluntary verbs.
   1.5. **Near-moment nudge for the voluntary verbs.** Between deterministic delivery and voluntary recall: a PostToolUse/Stop hook detects memory-worthy moments and escalates the highest-value ones to pointed `tm_propose`/`tm_observe` prompts, while the verbs themselves stay voluntary.
2. **Memory must affect future behavior.** If a record does not help a future agent decide better, it is not TeamMemory material.
3. **Agents propose and observe naturally.** Collaboration happens during normal work, not in review rituals — and never waits on human code review.
4. **Memories earn trust through independent evidence.** Active ≠ authoritative; contradictions weaken; staleness is detected, not assumed away.
5. **High-risk enforcement requires governance.** Agents discover rules; humans promote them to requirements.
6. **Git is the substrate.** Append-only records on an orphan branch: auditable via `git log`, synced via fetch/push, conflict-free by construction.
7. **Context files are projections.** `AGENTS.md`, `CLAUDE.md`, and `.cursor/rules` blocks are generated outputs, never the source of truth.
8. **Precision over recall.** A small number of highly relevant memories beats context spam. Output caps are a feature.

## 7. Architecture

### 7.1 Storage: Orphan Branch

`tm init` creates an orphan branch (default name `teammemory`) in the code repo:

```text
teammemory branch:
  memories/01J8X4QZ7M9FKE2V3R5T8WYBCD.yaml
  observations/01J8X5A2P4HND7QW9XK1MZRTGE.yaml
  policy.yaml
```

* Same remote as the code repo: no second repo to provision, no extra permissions, new teammates get it with the clone.
* Code branches stay clean: memory never appears in code diffs or PRs.
* Memory syncs independently of code-branch merge cadence: a memory proposed on one feature branch is visible to agents on other branches seconds later.
* No human PR review gates memory evolution; governance is `approve`/`reject` observations via the CLI.

`tm` reads and writes the branch through git plumbing (`hash-object`, `mktree`, `commit-tree`, `update-ref`) — no visible checkout, nothing ever touches the user's working tree.

**Separate-remote mode:** a single git config key (`git config tm.remote git@github.com:acme/billing.memory.git`) points the branch at a different remote for teams with strict branch protection. Same code path, different push target. Not the default.

### 7.2 IDs and Conflict-Freedom

All record IDs are ULIDs (sortable, collision-free across machines). Filenames are `<ulid>.yaml`. Because records are append-only and filenames are globally unique, concurrent pushes from any number of agents can never produce a merge conflict. `tm sync` resolves divergent branch tips by creating a merge commit containing the union of records (a tree-level union; no textual merging is ever needed).

### 7.3 Local Index

A derived SQLite index lives at `.git/tm/index.db` (inside `.git/`, so it can never be committed). It holds materialized derived state and an FTS table over titles/summaries/guidance. It is rebuilt by full replay (`tm reindex`, also run by `tm init`) and updated incrementally on sync. Corruption or version mismatch triggers an automatic rebuild — the branch is always the source of truth.

Session-local state (requirement acknowledgments, last-fetch timestamp) lives in `.git/tm/` as well and is never synced.

### 7.4 Sync Model

* `tm sync` = fetch + union-merge + push of the `teammemory` branch only.
* **Opportunistic background fetch:** `check_action` triggers a non-blocking background fetch when the last fetch is older than `sync.auto_fetch_after` (default 5 minutes). Fresh memories flow between agents without anyone running `tm sync` manually. The hook never waits on the network.
* Push happens on `propose`/`observe` (best-effort, async) and on explicit `tm sync`. Offline operation queues locally and pushes on next sync.
* **Concurrent-push resilience:** if the remote ref is advanced or created between a sync's fetch and its push — by the async background push, or by another clone — the push is rejected; `tm sync` re-fetches, re-reconciles (union-merge as needed), and re-pushes, up to a small bounded number of attempts. Since records are append-only ULID files, every attempt converges.

## 8. Derived State Specification

The function `f(memory, observations[], policy)` computes the following, in order. It must be implemented as a pure function with golden-file tests.

### 8.1 Risk

```text
risk = escalate(base_risk[memory.type], memory.effective_scope, policy)
```

1. Base risk from the type table in `policy.yaml`. A `constraint` with `origin: external` takes base risk `high`.
2. Escalators (applied to the highest-matching level):
   * scope breadth: a scope is broad if any of its globs can match paths in more than one top-level directory (e.g. `**`, `*/**`); broad scope escalates one level;
   * command breadth: a command scope is broad if any of its command patterns has one or fewer fixed leading tokens (a bare-binary pattern such as `assistant *` or `assistant`, as opposed to a subcommand-qualified one like `assistant jira create *`); broad command scope escalates one level. There are no sensitive-command escalators in v1.
   * sensitive paths: if the scope intersects any configured sensitive-path glob, risk is raised to at least that glob's level.

Default `policy.yaml`:

```yaml
base_risk:
  stale_doc: low
  decision: low
  failed_attempt: medium
  fragile_area: medium
  constraint: medium    # origin: external escalates to high

escalators:
  broad_scope_bump: true
  sensitive_paths:
    - glob: "**/migrations/**"
      min_risk: high
    - glob: "**/auth/**"
      min_risk: critical
    - glob: ".github/workflows/**"
      min_risk: critical

activation:
  independence: different_session   # or: different_session_and_branch
  tiers:
    low:      { auto: immediate,           max_auto_enforcement: recommendation }
    medium:   { auto: independent_confirm, max_auto_enforcement: warning }
    high:     { auto: independent_confirm, max_auto_enforcement: warning }
    critical: { auto: independent_confirm, min_independent_confirms: 2, max_auto_enforcement: warning }  # 2 confirms; requirement still human-only

requirement_enforcement:
  human_required: true

retrieval:
  max_results: 5
  max_provisional: 2
  provisional_mode: related   # never | related | always

sync:
  auto_fetch_after: 5m

nudge:
  enabled: true
  max_per_session: 3      # hard ceiling on nudges emitted per session
  cooldown_turns: 3       # min turns between two nudges
  self_review_every: 8    # turns before a generic memory-worthiness self-review
  churn_threshold: 3      # edits to one path before it counts as a fragile-area churn signal
```

Agents never supply risk or confidence. The proposing agent supplies only type, content, scope, evidence, and anchors.

### 8.2 Status

Evaluated in precedence order:

1. **rejected** — a human `reject` observation exists. Terminal.
2. **stale** — a `mark_stale` observation exists with no newer `confirm` or `approve`. Excluded from retrieval; listed by `tm list --stale`.
3. **contested** — a `contradict` observation exists with no newer `confirm` or `approve`. Dropped from active retrieval; surfaced only as caution (provisional framing), flagged for human attention in `tm status`.
4. **active** — per the risk tier:
   * `low`: active immediately on creation;
   * `medium` / `high`: active once ≥1 *independent* `confirm` exists, or a human `approve` exists;
   * `critical`: active once ≥2 *independent* `confirm`s exist, or a human `approve` exists.
5. **provisional** — otherwise.

**Independence (default):** an observation is independent if its `actor.session_id` is present and differs from the memory's `actor.session_id`. The stricter `different_session_and_branch` mode additionally requires the observation's `code_context.branch` to differ from the memory's `code_context.branch`; if either branch is absent, this mode degrades to session-only.

### 8.3 Confidence

* `low` at creation.
* `medium` after 1 independent confirmation.
* `high` after 2+ independent confirmations, or any human `approve` (which may also set it explicitly).
* Each contradiction — resolved or not — steps the computed level down one (floor: `low`). Unresolved contradictions additionally force `contested` status per 8.2.

### 8.4 Enforcement

* Auto-activated memories take their risk tier's `max_auto_enforcement` — enforcement is risk-proportional (louder in riskier areas): `recommendation` for low risk, `warning` for medium/high. Provisional memories surface as `hint`.
* A human `approve` observation may set enforcement explicitly, up to `requirement`.
* `requirement` is reachable **only** via human `approve`.

### 8.5 Effective Scope

* Starts as the envelope scope (path globs).
* An `adjust_scope` that **narrows** (suggested scope ⊆ current effective scope) applies immediately — narrowing only reduces noise.
* An `adjust_scope` that **broadens** recomputes risk at the suggested scope and applies only once substantiated: either (a) a human `approve` exists after it, or (b) a later independent `confirm` has a code context matching the suggested scope but not the prior effective scope — evidence the lesson really does apply beyond its original bounds. Until then it is pending and visible in `tm show`.
* Latest applicable adjustment wins.
* Effective scope covers command patterns as well as paths; an `adjust_scope` may narrow or broaden them. Command-pattern containment is by token-prefix (`assistant jira create *` ⊆ `assistant *`). A command broadening is substantiated like a path broadening: by a human `approve`, or by a later independent `confirm` whose `code_context.commands` match the broader pattern but not the prior one.

### 8.6 Anchor Drift (v1, cheap version)

At `check_action` time, for each anchored path: does the path still exist, and has the file changed since the memory's anchored commit (`git log --oneline <commit>.. -- <path> | wc -l`)?

Drift never auto-changes status. Drifted memories are **annotated** in output:

> "Note: anchored file has changed 14 commits since this memory was recorded — verify it still applies, and `mark_stale` if not."

Heavy drift surfaces in `tm list --stale-candidates` for cleanup.

## 9. Data Model

### 9.1 Memory Record (immutable)

```yaml
# memories/01J8X4QZ7M9FKE2V3R5T8WYBCD.yaml
id: 01J8X4QZ7M9FKE2V3R5T8WYBCD
type: failed_attempt
title: Billing migrations require downgrade-path tests
summary: >
  A previous attempt to modify invoice-state migration logic failed rollback
  because no downgrade path was included.
guidance: >
  Before modifying billing migrations, check rollback behavior and add
  downgrade-path tests.
scope:
  paths:
    - "billing/migrations/**"
  commands:               # optional: command patterns this memory applies to
    - "alembic upgrade *"
evidence:
  - type: test_failure
    description: rollback failure log
    ref: "logs/rollback_failure.log"
anchors:
  - path: "billing/migrations/2026_add_invoice_state.sql"
    commit: abc123def
code_context:           # optional: where the memory was proposed
  branch: feature/invoice-state
  commit: abc123def
  commands:             # optional: commands the proposing agent was running
    - "alembic upgrade heads"
actor:
  kind: agent            # agent | human
  name: claude-code
  session_id: session_123
created_at: "2026-06-15T10:00:00Z"
```

Repo scoping is implicit (the ledger lives in the repo). `services`, line ranges, symbols, and content hashes are roadmap.

### 9.2 Observation Record (immutable)

```yaml
# observations/01J8X5A2P4HND7QW9XK1MZRTGE.yaml
id: 01J8X5A2P4HND7QW9XK1MZRTGE
target: 01J8X4QZ7M9FKE2V3R5T8WYBCD
kind: confirm            # confirm | contradict | adjust_scope | mark_stale | approve | reject
summary: >
  Same rollback failure reproduced on revenue-reporting branch.
evidence:
  - type: test_failure
    description: rollback failure on second branch
    ref: "logs/revenue_rollback_failure.log"
code_context:
  branch: feature/revenue-reporting
  commit: def456abc
  paths:                 # optional: files the observing agent was working on
    - "billing/migrations/2026_add_invoice_state.sql"
  commands:              # optional: commands the observing agent ran
    - "alembic upgrade heads"
actor:
  kind: agent
  name: codex
  session_id: session_456
created_at: "2026-06-15T11:20:00Z"

# kind-specific fields:
# adjust_scope:  suggested_scope: { paths: [...] }
# approve:       set_enforcement: requirement   set_confidence: high   (both optional)
```

## 10. Integration Surface

### 10.1 Claude Code Plugin (flagship, ships in v1)

The plugin installs three things:

**PreToolUse hook** on `Edit|Write|MultiEdit|Bash`: runs `tm check-action --hook` against the local SQLite index. Budget: under 100ms end-to-end (no network; background fetch is detached).

For edit tools (`Edit|Write|MultiEdit`), the hook reads `tool_input.file_path` and matches memories by path scope. For `Bash` tool calls, the hook reads `tool_input.command` and matches memories by `scope.commands`. An unacknowledged active `requirement` memory whose command pattern matches the Bash command **denies the command** (same block-and-ack flow as edits).

v1 limits: command matching uses leading-subcommand matching only (flags are not matched). Shell composition (pipes, `&&`, subshells) is best-effort.

* Matching `hint` / `recommendation` / `warning` memories → injected as additional context on the tool call.
* Matching unacknowledged `requirement` memories → the hook **denies the edit or command**, returning the guidance and required checks as feedback:

> "Requirement (mem 01J8X4…): Billing migrations require downgrade-path tests. Run the downgrade-path tests, then run `tm ack 01J8X4…` and retry."

**SessionStart hook**: runs `tm brief` at session start; stdout is injected as session context. The briefing carries live ledger counts plus the standing instructions for the voluntary verbs — deterministic delivery of *when to remember*, not just *what is remembered*.

**PostToolUse hook** runs `tm signal --hook`: records raw events — command outcomes and edits — into a per-session journal under `.git/tm/nudge` (on non-Claude harnesses it also records surfaced memories; on Claude those come from the PreToolUse `check-action` hook). Silent; never blocks. Each event advances a within-session turn counter.

**Stop hook** runs `tm nudge --hook`: at turn end, it reads the journal, detects the memory-worthy signals (fail→pass, revert, edit churn, surfaced-but-unobserved, drift-anchor), and emits at most one proposing/observing nudge per the anti-spam policy (max 3/session, cooldown 3 turns, suppress-if-already-acted, observe outranks propose). Low-pressure wording; the verbs stay voluntary. The nudge is injected as additional context, not as a forced turn.

**UserPromptSubmit hook** records a prompt marker into the journal so the user-intervened signal can detect edit→prompt→re-edit on the same path (a Tier B attention flag that only aims the periodic self-review).

The nudge journal is local state under `.git/tm/nudge`, never a ledger record — like acks (Section 10.2). Detection is pure and the suppress-if-acted check is injected as a ledger predicate, so the nudge engine carries no index/ledger coupling.

**MCP server** (stdio) for the voluntary verbs — see 10.3.

### 10.2 Requirement Acknowledgment

`tm ack <memory-id> [--note "..."]` records an acknowledgment in `.git/tm/` (local-only). It is keyed by session ID when one is available (the hook receives it from Claude Code; `tm ack` reads `CLAUDE_SESSION_ID` if set), otherwise it falls back to a TTL (default 8h, configurable). Subsequent edits matching that memory pass while the ack is live. Acks are local state, not ledger records — they gate the hook, they are not evidence.

### 10.3 MCP Tools

1. `tm_check_action` — input: action description, paths, optional `command` (matched against memory `scope.commands`), optional provisional mode. Output: active memories, strongly-related provisional memories (capped, caution-framed), drift annotations, requested observations. For pre-task planning; the hook covers edit time.
2. `tm_propose` — create a memory. Tool description constrains usage to durable, future-action-relevant project judgment and enumerates memory-worthy events (non-obvious failure, hidden constraint, stale doc, fragile area, undocumented decision) and non-events (session state, trivia, code facts derivable from the repo, system/OS-specific facts). Accepts path globs via `scope` and command patterns via `commands` (e.g. `pytest *`).
3. `tm_observe` — add `confirm` / `contradict` / `adjust_scope` / `mark_stale` with evidence. Tool description: observe when your work bears on a memory you were shown — confirmation with evidence, contradiction with evidence, scope correction, or staleness. `adjust_scope` accepts `scope` (path globs), `commands` (command patterns), or both.
4. `tm_search` — lexical search over the ledger.
5. `tm_status` — counts of active/provisional/contested/stale, pending human items, sync state.

Sync is not an MCP tool — it is automatic (Section 7.4).

### 10.4 Other Agents (Cursor, Codex, Continue)

Same MCP server. As of 2026, Codex CLI, Copilot CLI, Cursor, Gemini CLI, and Continue CLI all support session-start hooks with context injection; `tm brief --format <tool>` emits the briefing in each tool's envelope (setup snippets in the README), so the session-start instruction path works everywhere. `tm export` still generates instruction blocks for `AGENTS.md` and `.cursor/rules` — including usage preambles for the three verbs — as a fallback for surfaces without hooks (e.g. IDE extensions). Projections are clearly marked generated artifacts.

### 10.5 CLI

Seventeen commands:

```text
tm init          # create orphan branch, default policy.yaml, local index;
                 # install Claude Code hooks+MCP, or --harness codex|copilot|cursor|gemini
tm sync          # fetch + union-merge + push the teammemory branch
tm check-action  # query memory for an action (--hook mode for the plugin)
tm signal        # record nudge signals from a PostToolUse event, or a prompt marker (--hook; --prompt; --harness)
tm nudge         # emit at most one near-moment nudge from a Stop event (--hook; --harness)
tm brief         # session-start briefing for agent hooks (live counts + instructions)
tm propose       # create a memory record
tm observe       # add an observation record
tm ack           # session-scoped requirement acknowledgment (local-only)
tm approve       # human: activate / raise enforcement or confidence
tm reject        # human: kill a memory
tm list          # list memories (--stale, --contested, --stale-candidates)
tm show          # full detail: envelope, observations, derived state, pending scope adjustments
tm search        # lexical search
tm export        # generate AGENTS.md / CLAUDE.md / .cursor/rules blocks / JSON
tm status        # ledger overview, items needing human attention, sync state
tm doctor        # validate setup: ledger branch, index, hooks, MCP, remote
```

`tm approve` and `tm reject` write `approve`/`reject` observation records with `actor.kind: human`.

### 10.6 Cross-harness adapters

The hook engine is harness-neutral. A single `Event`/`Decision` model (`internal/harness`) expresses what a hook saw (a pre-tool action, a post-tool outcome, a turn end, a prompt) and what to do about it (block with a reason, inject advisory context, or nothing). Each supported agent has a thin **adapter** that parses its concrete hook payload into an `Event` and renders a `Decision` back into its wire format; the engine (retrieval, requirement enforcement, nudge) never sees harness-specific JSON. Adding a harness is one adapter plus its packaging — no engine changes.

The three hook verbs — `check-action` (PreToolUse), `signal` (PostToolUse / UserPromptSubmit), and `nudge` (Stop) — take `--harness <name>` (default `claude`) to select the adapter. On `UserPromptSubmit`, `signal` runs with `--prompt`: it records a prompt marker and advances the turn clock so the user-intervened signal can detect edit→prompt→re-edit on the same path; the PostToolUse path records command/edit outcomes.

Per-harness wire shapes (v1 ships Claude Code, Codex, Copilot, Cursor, and Gemini):

| Harness | Event names | Block / inject output | Outcome (fail) source |
| --- | --- | --- | --- |
| Claude Code | `PreToolUse` / `PostToolUse` / `Stop` / `UserPromptSubmit` | `hookSpecificOutput.{permissionDecision,additionalContext}` | none — `PostToolUse` fires on success only (no failure event, no `exit_code`); `PostToolFailureSensor = no` |
| Codex | same names, same `hookSpecificOutput` shape | same | none — `PostToolUse` fires on success only; `PostToolFailureSensor = no` |
| Copilot | `preToolUse` / `postToolUse` / `errorOccurred` / `userPromptSubmitted` / `agentStop` | bare `{permissionDecision}` / `{additionalContext}` | `errorOccurred` event / `error` field / non-zero `exit code N` parsed from `toolResult.textResultForLlm` (live shell shape; `toolResult.exitCode` retained forward-compat) |
| Cursor | `beforeShellExecution` / `afterShellExecution` / `postToolUseFailure` / `afterFileEdit` / `beforeSubmitPrompt` / `stop` | `{permission,agent_message}` (block) / `{additional_context}` (allow) | `postToolUseFailure` event (failure flag, no exit code); for a failed shell command the command is nested at `tool_input.command`, not top-level |
| Gemini | `BeforeTool` / `AfterTool` / `BeforeAgent` / `AfterAgent` | `{decision,reason}` (block) / `{hookSpecificOutput:{additionalContext}}` (allow) | non-zero `Exit Code: N` line in `tool_response.llmContent` (live shell shape; `tool_response.error` retained forward-compat) |

**Packaging.** `tm init` (default `claude`) installs the Claude Code hooks into `.claude/settings.json` and registers the `teammemory` MCP server in the repo-root `.mcp.json` (merge-safe). `tm init --harness {codex,copilot,cursor,gemini}` writes the harness's hook and plugin artifacts and registers MCP automatically: Codex gets `<repo>/.codex/hooks.json` (the event map wrapped under a top-level `hooks` key — the path Codex actually loads; the legacy `.codex-plugin/` layout only loads as a marketplace-installed, trusted plugin) plus an `[mcp_servers.teammemory]` table appended to `~/.codex/config.toml`; Copilot gets `.github/hooks/teammemory.json` (each hook carrying both `bash` and `powershell` command keys so it runs cross-OS) plus a merged `teammemory` entry in `~/.copilot/mcp-config.json`; Cursor gets `.cursor/hooks.json` plus `.cursor/rules/teammemory.mdc` plus a merged `.cursor/mcp.json`; Gemini gets `.gemini/settings.json` (hooks + MCP) plus a `GEMINI.md` section. MCP registration merges into any existing config — existing servers, hooks, and top-level keys are preserved — so it is safe to re-run, including Gemini's combined `.gemini/settings.json`. Codex and Copilot register MCP in the user's home directory because that is where those CLIs read it; every other artifact is repo-local.

**Advisory injection fidelity.** Requirement enforcement (PreToolUse block + ack) works identically on every harness. Advisory (`hint`/`recommendation`/`warning`) memories differ in *when* they surface: Claude Code injects them **pre-edit** via the `check-action` hook, while other harnesses inject them **post-edit** via the `signal` hook (those harnesses only inject context post-tool). Post-edit injection retrieves advisory memories for the edited path, skips requirements (still blocked pre-tool), dedups per session, and is capped by `inject.advisory_max_per_session` (default 5). This pre-vs-post timing is the one deliberate fidelity difference between Claude Code and the other harnesses.

Codex hook discovery (`<repo>/.codex/hooks.json`, wrapped schema) and `apply_patch` coverage are confirmed against OpenAI's published hook docs — `apply_patch` is the file-edit tool (a matcher may also name `Edit`/`Write`, but the hook input always reports `tool_name: "apply_patch"`). The `tool_response.exit_code` path was taken from those docs but live payloads diverge (see the codex live caveat below): a successful shell `PostToolUse` carries `tool_response` as a plain string, and failing tool calls emit no `PostToolUse` at all. Copilot's hook location (`.github/hooks/*.json`), the required `bash`+`powershell` command keys, and the event set (`errorOccurred`, not `postToolUseFailure`) are confirmed against GitHub's docs. Cursor's field names are confirmed against live `cursor-agent` payloads (cursor 2026.06.12): `afterShellExecution` carries top-level `command` + `output` with **no exit code** (it fires for both passing and failing commands), while a failed shell command's failure surfaces via a separate `postToolUseFailure` event whose command is nested at `tool_input.command` (with `tool_name`/`error_message`/`failure_type`); a non-shell failure (e.g. a failed `Read`) carries `tool_input.file_path` and no command, so it is not a command outcome. The headless `cursor-agent` CLI does **not** fire the `stop` or `beforeSubmitPrompt` hooks, so Cursor's nudge (Stop) and prompt signals are exercised only in the IDE or via replayed fixtures — not by live capture. On Windows, `cursor-agent` also prepends a UTF-8 BOM to hook stdin, which Go's JSON decoder rejects (`invalid character 'ï'`) — silently breaking every cursor hook; every adapter now strips a leading BOM before decoding (`harness.decodeJSON`), so cursor's signals/nudges/blocks work on Windows. Gemini's hook config must use the nested group shape — each event holds an array of `{ "matcher": <regex>, "hooks": [{ "type": "command", "command": … }] }` groups (tool events require a matcher; `BeforeAgent`/`AfterAgent` omit it). A flat `[{ "command": … }]` entry is rejected at load ("Discarding invalid hook definition") and never fires; `tm init --harness gemini` writes the nested shape. Confirmed against live `gemini` payloads: `hook_event_name` is `BeforeTool`/`AfterTool`/`BeforeAgent`/`AfterAgent`, the shell tool reports `tool_name: "run_shell_command"` with the command at `tool_input.command`, and a `.*` matcher fires for every tool. Live capture (2026-06-15) resolved the per-harness command-failure shapes — each diverges from the structured `exit_code` originally assumed, and the assumed fields were never present, so failure detection was silently broken for all three until this pass. **Copilot** (1.0.62) fires `postToolUse` on a failed shell command but reports `toolResult.resultType: "success"` with no `exitCode` field — the real exit status is the trailing `exit code N` in `toolResult.textResultForLlm` (e.g. `<shellId: 0 completed with exit code 1>`), which the adapter now parses. **Gemini** fires `AfterTool` on failure but does not populate `tool_response.error`; the exit status is the `Exit Code: N` line in `tool_response.llmContent` (present only on a non-zero exit — a successful command's `llmContent` has no such line), which the adapter now parses. **Claude Code** (2.1.177), like Codex, fires `PostToolUse` only on tool *success* — a failing Bash command emits `PreToolUse` then no `PostToolUse`, and even a successful Bash `tool_response` (`{stdout,stderr,interrupted,isImage,noOutputExpected}`) carries no `exit_code` — so command-failure sensing cannot fire, and the matrix sets claude `PostToolFailureSensor = no` (so claude's `fail_pass_nudge` scenario is not applicable, like codex's). **Re-check both Claude and Codex by ~2026-08-15**: re-run a failing command; if a later version emits a `PostToolUse` (or a dedicated error event) on failure, re-enable `PostToolFailureSensor` and restore the `fail_pass_nudge` scenario for that harness. The last open live-payload checks — Copilot's and Gemini's `additionalContext` model-visibility — are now **verified** (live probe, 2026-06-16): a hook's `additionalContext` reaches the model on both. Gemini surfaces it directly; Copilot surfaces and trusts **informational** advisory content (the shape tm actually injects — naming a relevant memory) but flags an *imperative* instruction delivered through the hook as a prompt-injection attempt and declines it. tm's advisory injection is informational, not imperative, so this is a design guardrail to preserve (keep advisory context descriptive), not a defect. (This is verified by one-time probe rather than a CI gate: it depends on live model behavior, which is non-deterministic and would make a committed assertion flaky; the recipe is in `docs/verification/cross-harness.md`.) The harness test suite (fixtures plus build-tag-gated live smoke tests) pins the deterministic wire-shape and behavior checks. That suite now exists: the default tiers (contract/replay/packaging) run on committed authored fixtures, and a `//go:build harness_live` overlay adds a live-firing tier (`TestLive`), live **behavior** tests (`TestLiveRequirementBlock` installs real tm, seeds an active requirement, and asserts a live edit to the protected path is blocked — the file stays unwritten AND the requirement is surfaced in the nudge journal, which together prove the hook fired and the harness honored the deny; capability-gated and verified live for **all five harnesses** as of 2026-06-16, confirming each honors a hook deny even under its permission-bypass run flag (`--dangerously-skip-permissions`/`--allow-all-tools`/`--yolo`/`--force`/`--dangerously-bypass-approvals-and-sandbox`). Cursor uses a **command-scoped** requirement (its pre-tool block is shell-only — no pre-edit hook). **Codex** required two fixes this test uncovered: (a) its file-edit tool is `apply_patch`, which carries the edited path inside the patch text at `tool_input.command` (`*** Add/Update/Delete File: <path>`), not a `file_path` field — `codex.go` now parses that path, so path-scoped blocking and advisory injection match codex edits (previously the patch blob was mis-recorded as a *command* and no path matched, so codex file-edit blocking silently no-opped — the codex `requirement_block`/`edit` fixtures were authored as a `tool_name:"Edit"`+`file_path` shape that codex never emits, masking the gap); (b) `check-action` now resolves a repo-relative hook path against the repo root (codex emits relative `apply_patch` paths) via the shared `relPath` helper, rather than `filepath.Abs` against the process CWD. Codex's live block test is gated on its one-time interactive hook trust (scaffold via `TestSetupCodexBlockRepo` + `TM_CODEX_BLOCK_REPO`, then run `TestLiveCodexRequirementBlock`); `TestLiveRealTmRecording` installs real tm and asserts it records the command/edit into `.git/tm/nudge` across the per-run harnesses), and a capture tier (`TestCapture`) that drive the real CLIs. Live firing is confirmed for **Claude, Copilot, Cursor, and Gemini** (all four load and run the installed hook headless). One live caveat is recorded: (1) Codex hooks **do** fire — including headless `codex exec` — but only after the repo's hooks are **trusted once interactively** (`codex` TUI → "Hooks need review" → "Trust all and continue"), which persists a per-hook `sha256` under `[hooks.state]` in `~/.codex/config.toml` keyed by `<hooks.json path>:<event>:<idx>:<idx>`. With that trust in place, `codex exec` fires `SessionStart`/`UserPromptSubmit`/`PreToolUse`/`PostToolUse`/`Stop` (verified 2026-06-15, CLI 0.139.0; `tool_name: "Bash"` for the shell tool, confirming the `^(Bash|apply_patch)$` matcher). The blocker for the automated `TestLive/codex` is that each run uses a fresh, untrusted temp repo, and `--dangerously-bypass-hook-trust` does **not** substitute for the persisted trust in this version — so Codex's automated live test is skipped/gated on a one-time-trusted repo rather than run per-invocation. Two codex wire-shape findings (correcting the doc assumption above): (a) a successful `PostToolUse` carries `tool_response` as a **string** (the command output), not `{exit_code: …}` — `codex.go` now tolerates both shapes (string ⇒ passing outcome; object ⇒ exit-code check retained for forward-compat); (b) `PostToolUse` fires on tool **success only** — a **failing** tool call emits **no `PostToolUse` at all** (only `PreToolUse` then `Stop`), confirmed in **both** headless `codex exec` and interactive `codex` (CLI 0.139.0). Command-failure sensing therefore cannot fire on codex in any mode, so the capability matrix sets codex `PostToolFailureSensor = no` and the codex `fail_pass_nudge` scenario is not applicable. (This differs from Cursor, whose Stop/prompt gap is headless-only and so stays `yes`+documented; codex's failure gap is fundamental to its hook model.) The adapter retains exit_code-object parsing for forward-compat should a future codex emit a failure `PostToolUse`. **Re-check by ~2026-08-15** (codex's hook surface is moving fast): re-run a failing command under codex; if a later version emits a `PostToolUse` (or a dedicated error event) on failure, re-enable codex `PostToolFailureSensor` and restore the `fail_pass_nudge` scenario; (2) the capture tier normalizes a captured payload into a deterministic, committable fixture — it pins the session id, rewrites the repo root (including Windows JSON-escaped backslash paths) to a forward-slash `{{REPO}}` path, and strips volatile per-run fields (claude's `transcript_path`/`tool_use_id`/`duration_ms`) — and re-emits it compactly without HTML-escaping shell metacharacters; the repo still ships the authored fixtures by default, with `task capture:<harness>` available to refresh them from a live run (the picked payloads are diff-reviewed before commit).

The scenario-capability matrix below is the authoritative source for which E2E scenarios apply to each harness. The harness E2E suite parses this exact fenced block and fails if a descriptor disagrees (see e2e/harness/conformance_test.go).

```capability-matrix
harness | PreToolBlock | PostToolFailureSensor | StopNudge | PromptSubmit | AdvisoryInjection
claude  | yes          | no                    | yes       | yes          | no
codex   | yes          | no                    | yes       | yes          | yes
copilot | yes          | yes                   | yes       | yes          | yes
cursor  | yes          | yes                   | yes       | yes          | yes
gemini  | yes          | yes                   | yes       | yes          | yes
```

## 11. Retrieval

V1 is precision-first, lexical only. No embeddings.

1. **Candidate set:** memories enter as candidates through three channels:
   - **Scope channel:** effective scope globs match one or more of the action's paths.
   - **Command channel:** effective command patterns match the action's command. Matching is token-aware: leading subcommand tokens must match literally, a trailing `*` matches the rest of the command; flags are not matched. Example: `pytest *` matches `pytest -q tests/`; `assistant jira create *` matches `assistant jira create FOO-1` but not `assistant jira delete FOO-1`.
   - **FTS channel:** the action description matches title/summary/guidance (for path-less and command-less queries like planning checks).
2. **Ranking:** specificity of the best structural match (scope or command) > status (active first) > enforcement > confidence > recency > anchor freshness (drifted memories rank lower). Command-match specificity is computed like glob specificity: base 1, plus 2 per fixed (non-wildcard) token; more fixed tokens ⇒ higher specificity ⇒ ranks higher.
3. **Provisional inclusion** (`provisional_mode`, default `related`): provisional and contested memories appear on any structural match (scope-glob or command-pattern), but not on FTS-only matches. Capped at `max_provisional` (default 2), always caution-framed, always with a requested-observation prompt.
4. **Output cap:** `max_results` (default 5) total. Context spam is a top product risk; the cap is a feature.

Symbol matching, error-signature matching, and semantic ranking are roadmap.

## 12. MVP Scope

### 12.1 Must Have

1. Open-source repo, docs, and a quickstart that works in under 10 minutes.
2. Go CLI (all 13 commands).
3. Orphan-branch ledger via git plumbing; ULID record files; union-merge sync.
4. Local SQLite index with full-replay rebuild and incremental update.
5. Derived-state function exactly per Section 8, with golden-file tests.
6. Deterministic risk computation with configurable `policy.yaml`.
7. MCP server (5 tools) with usage-constraining descriptions.
8. Claude Code plugin: PreToolUse hook (inject + requirement-block + ack), MCP registration.
9. Opportunistic background fetch.
10. Anchor-drift annotation.
11. `tm export` projections for `AGENTS.md`, `CLAUDE.md`, `.cursor/rules`, JSON.
12. Flagship demo (Section 13) runnable from a script.
13. Tests: lifecycle golden files, retrieval precision cases, two-clone concurrent-sync (zero conflicts), hook latency budget.
14. Session-start briefing (`tm brief`) with per-tool envelope formats; installed as a Claude Code SessionStart hook by `tm init`.

### 12.2 Nice to Have

1. `tm doctor` (validate branch, index, hook installation). **Shipped** — see §10.5.
2. Static HTML timeline report.
3. Homebrew/Scoop packaging.

### 12.3 Explicitly Out of Scope for MVP

1. SaaS backend, web dashboard, GitHub app.
2. Embeddings / semantic retrieval.
3. Event-sourcing ledger with materialized state (superseded by derived state).
4. `ownership` / `successful_pattern` types; `mark_duplicate` / `suggest_supersession` observations.
5. Signed records, multi-human approval.
6. Symbol-level anchors, content-hash anchor resolution.
7. Dedicated reviewer agents, multi-agent debate.

## 13. Flagship Demo

**Ambient Memory Validation Across Branches**

Scripted against a seeded fake `billing-service` repo.

1. **Agent A** on `feature/invoice-state` modifies a billing migration, hits a rollback failure, and proposes via MCP:

```bash
tm propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --scope "billing/migrations/**" \
  --summary "Rollback failed when invoice_state migration lacked downgrade path." \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor "billing/migrations/2026_add_invoice_state.sql@HEAD"
```

Derived state: risk `high` (base `medium`, escalated by the `**/migrations/**` sensitive path), status `provisional`, confidence `low`.

2. **Agent B** on `feature/revenue-reporting` opens a related migration file. **The PreToolUse hook fires** — B never decides to check — and surfaces the provisional memory as caution with a requested observation. B runs the downgrade tests, reproduces the failure, and confirms via MCP:

```bash
tm observe 01J8X4QZ7M9FKE2V3R5T8WYBCD confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --evidence "test_failure:logs/revenue_rollback_failure.log"
```

3. **Auto-activation:** high tier + 1 independent confirmation ⇒ status `active`, confidence `medium`, enforcement `warning`.

```text
Memory 01J8X4QZ… activated automatically.
  risk: high   confidence: medium   enforcement: warning
  reason: 1 independent confirmation (different session, different branch)
```

4. **Human escalation:**

```bash
tm approve 01J8X4QZ7M9FKE2V3R5T8WYBCD --enforcement requirement --confidence high
```

5. **Enforcement demo:** a third agent attempts to edit a billing migration; the hook **blocks** the edit with the guidance, the agent runs the downgrade tests, acks, and proceeds.

The demo shows the deterministic path (hook), not the voluntary one — and every step of the ledger is inspectable with `git log teammemory -- memories/ observations/`.

## 14. Success Metrics

### 14.1 Product Success

1. New user completes the quickstart and demo in under 10 minutes.
2. Hook check completes in under 100ms on a ledger of 1,000 memories.
3. The full lifecycle works end-to-end: propose → provisional surface → independent confirm → auto-activate → human escalation → requirement block.
4. Two clones proposing/observing concurrently sync with zero merge conflicts.
5. **Trap-repo benchmark:** in a seeded repo with a known pitfall, a naive agent repeats the mistake and a TeamMemory-equipped agent avoids it. This is the honest "does memory prevent repeated mistakes" metric, and a demo asset.

### 14.2 Open-Source Success

First 30 days: 100 GitHub stars; 5 meaningful issues; 2 external users on real repos; 1 technical essay; 1 demo video.
First 90 days: 500 stars; 5 external contributors; documented setups for 2+ coding agents; 3 public references from AI/dev-tool communities.

## 15. Risks and Mitigations

**Agents ignore the tool** → the hook makes `check_action` deterministic in Claude Code (the headline feature, not a mitigation). Other agents: MCP + generated instructions; their experience is honestly documented as degraded. Session-start briefing injects the voluntary-verb instructions deterministically in every major agent CLI.

**Agents ignore the voluntary verbs** → (1) the SessionStart brief injects the when-to-remember instructions every session; (2) a near-moment nudge engine (PostToolUse signal recording + Stop emission, Section 10.1) escalates the highest-value moments to pointed `tm_propose`/`tm_observe` prompts, bounded by an anti-spam budget (max 3/session, cooldown, suppress-if-already-acted) so it never manufactures junk proposals.

**Memory spam** → usage-constraining MCP descriptions with explicit non-examples; five types only (`successful_pattern` deferred); provisional memories capped at 2 per check; cheap `reject`; FTS-assisted duplicate warning at propose time ("a similar memory exists — confirm it instead?").

**Bad memories poison agents** → active ≠ authoritative; independent confirmation gates medium+ activation; contradictions demote to contested immediately; requirement needs a human; anchor drift is annotated.

**Memory rot erodes trust** → anchor-drift annotation in v1; `mark_stale` is a first-class agent verb; `tm list --stale-candidates` for cleanup; drifted memories rank lower.

**Hook latency annoys users** → Go binary + local SQLite, <100ms budget enforced by a perf test; network never on the hook path; hook is removable independently of MCP.

**Branch protection blocks the orphan branch** → documented setup note (exempt `teammemory` from protection rules) and the separate-remote config fallback.

**Crowded market** → positioning per Section 2.1: evidence-validated + git-native + deterministic enforcement, against named alternatives.

## 16. Implementation Stack

* **Go** — single static binary, ~5–10ms cold start (fits the hook budget), trivial cross-platform distribution.
* CLI: `spf13/cobra`.
* SQLite: `modernc.org/sqlite` (pure Go, no cgo, simple cross-compilation; performance is ample at this scale).
* Git: shell out to the system `git` for plumbing and transport (git is guaranteed present — this is a git-based tool; avoids go-git edge cases with credentials/transports).
* YAML: `gopkg.in/yaml.v3`.
* MCP: official `modelcontextprotocol/go-sdk`.
* ULIDs: `oklog/ulid/v2`.
* Distribution: GitHub Releases via GoReleaser (cross-compiled archives + checksums on tag push) and `go install` — both live as of v0.0.1; curl install script and Homebrew/Scoop later.

## 17. Roadmap

**Phase 1 — MVP:** everything in Section 12.1. **Shipped** (v0.0.1: orphan-branch ledger, SQLite index, derived state, risk/policy, MCP server, Claude Code hook + nudge engine, `tm export`, retrieval, `tm brief`, `tm doctor`).

**Phase 2 — Breadth:** _in progress._

- **Cross-harness adapter layer** — Codex, Copilot, Cursor, and Gemini hook/MCP adapters with `tm init --harness {codex,copilot,cursor,gemini}` packaging (§6.2–§6.5, §10.6). **Shipped.**
- **Cross-harness E2E test framework** — per-harness payload fixtures (default contract/replay/packaging tiers) plus build-tag-gated live capture and live-firing tiers that pin the remaining live-payload checks (§10.6; `e2e/harness/`, recipes in `docs/verification/cross-harness.md`). **Shipped.**
- `ownership` and `successful_pattern` memory types; `mark_duplicate` and supersession observations.
- Polished separate-remote UX.
- **Package-manager distribution** — Homebrew and Scoop formulas on top of the existing GoReleaser pipeline, so `brew install teammemory` / `scoop install teammemory` are first-class install paths alongside `go install` and the GitHub Releases archives (§12.2, §16).

**Phase 3 — GitHub workflow:** bring memories onto the PR review surface, not just the live agent hook.

- **PR memory action** — a GitHub Action that runs retrieval against a PR's changed paths/commands and posts the relevant memories as a PR comment/check.
- **Memory timeline report** — the static HTML timeline (§12.2) visualizing the record/observation history over time.

**Phase 4 — Retrieval depth:** extend V1's precision-first lexical retrieval (§11) with deeper matching and ranking signals.

- **Symbol anchors** — anchor memories to code symbols (functions/types), not just file paths and line ranges (§9.1).
- **Error-signature matching** — surface memories by matching error/stack-trace signatures, so a failing command finds the memory about that failure.
- **Content-hash drift detection** — detect when anchored code *content* has changed via content hashes, beyond line-range drift (§9.1).
- **Semantic ranking** — embeddings-based ranking of candidates (V1 is lexical only, no embeddings — §11, §12.3).

**Phase 5 — Harness breadth:** extend the §10.6 adapter set to additional coding agents — one adapter plus its packaging per harness, no engine changes.

- **OpenCode** — hook + MCP adapter and `tm init --harness opencode` packaging.
- **Pi** — hook + MCP adapter and `tm init --harness pi` packaging.
- Further open-source coding agents as they gain a hook surface comparable to §10.6's matrix.

**Phase 6 — Governance depth:** signed records, multi-human approval, policy templates, expiry workflows, contested-memory review UI.

## 18. Decisions Locked

1. Open-source developer tool; core unit is future-action-relevant project judgment.
2. Ledger lives on an **orphan branch in the code repo**; separate remote is a config fallback, never a second code path.
3. Storage is **append-only records (memories + observations) with ULID filenames**; concurrent sync is conflict-free by construction.
4. **All state is derived**: status, confidence, risk, enforcement, and effective scope are a pure function of records + policy, cached in local SQLite, never stored or synced.
5. **Risk is policy-derived**, never agent-assessed.
6. **Tiered activation:** low risk activates immediately; medium/high need one independent confirmation; critical needs two independent confirmations; `requirement` enforcement always needs a human.
7. Five memory types: `failed_attempt`, `constraint`, `fragile_area`, `stale_doc`, `decision`.
8. Six observation kinds: `confirm`, `contradict`, `adjust_scope`, `mark_stale` (agents); `approve`, `reject` (humans).
9. **Hook-first integration via a shared engine + per-harness adapters:** one harness-neutral hook engine drives `check_action`, `requirement` enforcement (block + ack), and the near-moment nudge; thin adapters translate each agent's hook wire format (Claude Code, Codex, Copilot, Cursor, and Gemini in v1 — Section 10.6). The PreToolUse block path makes `requirement` enforcement real on every harness; advisory memories surface pre-edit on Claude Code and post-edit elsewhere — the one deliberate fidelity difference. MCP covers the voluntary verbs and hookless surfaces.
10. Memory evolution is autonomous between agents — no human code review in the loop; humans govern only activation of critical memories and requirement-level enforcement.
11. Anchor-drift annotation ships in v1.
12. Retrieval is precision-first and lexical in v1; output capped at 5 memories, 2 provisional.
13. Implementation in **Go**.
14. Session-start briefing is a first-class surface: `tm brief`, installed for Claude Code by `tm init`, with envelope formats for Codex, Copilot CLI, Cursor, Gemini CLI, and Continue.
