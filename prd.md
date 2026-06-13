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

Fifteen commands:

```text
tm init          # create orphan branch, default policy.yaml, local index;
                 # detect Claude Code and offer hook+MCP install; print MCP snippet
tm sync          # fetch + union-merge + push the teammemory branch
tm check-action  # query memory for an action (--hook mode for the plugin)
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

**Phase 1 — MVP:** everything in Section 12.1.

**Phase 2 — Breadth:** `ownership` and `successful_pattern` types; `mark_duplicate` and supersession; richer Cursor/Codex/Continue setup guides; polished separate-remote UX; `tm doctor`.

**Phase 3 — GitHub workflow:** GitHub Action surfacing relevant memories on code PRs; memory timeline report.

**Phase 4 — Retrieval depth:** symbol anchors, error-signature matching, content-hash drift detection, semantic ranking.

**Phase 5 — Governance depth:** signed records, multi-human approval, policy templates, expiry workflows, contested-memory review UI.

## 18. Decisions Locked

1. Open-source developer tool; core unit is future-action-relevant project judgment.
2. Ledger lives on an **orphan branch in the code repo**; separate remote is a config fallback, never a second code path.
3. Storage is **append-only records (memories + observations) with ULID filenames**; concurrent sync is conflict-free by construction.
4. **All state is derived**: status, confidence, risk, enforcement, and effective scope are a pure function of records + policy, cached in local SQLite, never stored or synced.
5. **Risk is policy-derived**, never agent-assessed.
6. **Tiered activation:** low risk activates immediately; medium/high need one independent confirmation; critical needs two independent confirmations; `requirement` enforcement always needs a human.
7. Five memory types: `failed_attempt`, `constraint`, `fragile_area`, `stale_doc`, `decision`.
8. Six observation kinds: `confirm`, `contradict`, `adjust_scope`, `mark_stale` (agents); `approve`, `reject` (humans).
9. **Hook-first integration:** Claude Code PreToolUse hook makes `check_action` deterministic and makes `requirement` enforcement real (block + ack); MCP covers voluntary verbs and other agents.
10. Memory evolution is autonomous between agents — no human code review in the loop; humans govern only activation of critical memories and requirement-level enforcement.
11. Anchor-drift annotation ships in v1.
12. Retrieval is precision-first and lexical in v1; output capped at 5 memories, 2 provisional.
13. Implementation in **Go**.
14. Session-start briefing is a first-class surface: `tm brief`, installed for Claude Code by `tm init`, with envelope formats for Codex, Copilot CLI, Cursor, Gemini CLI, and Continue.
