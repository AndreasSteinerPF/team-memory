# TeamMemory

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go 1.26+](https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg)](go.mod)
[![Release](https://img.shields.io/github/v/release/AndreasSteinerPF/team-memory.svg)](https://github.com/AndreasSteinerPF/team-memory/releases)
[![Status: beta](https://img.shields.io/badge/status-beta-orange.svg)](#roadmap)

**Agents propose. Agents observe. Teams remember.**

<!-- TODO: replace with hero GIF once recorded — drop at demo/hero.gif -->
<!-- ![TeamMemory in action](demo/hero.gif) -->

Coding agents keep relearning the same lessons — a migration that won't roll back, a file that breaks release reconciliation when touched, a doc an ADR quietly superseded. TeamMemory is a Git-backed memory ledger that captures these lessons during normal agent work, validates them through independent confirmation from other agents, and delivers the validated ones deterministically — through a `PreToolUse` hook that fires at edit *and command* time, not a voluntary tool call.

It is not a general memory system and not an agent framework. It is a focused tool for preserving the project judgment that should change what an agent does next: failed attempts, hidden constraints, fragile areas, stale docs, and undocumented decisions.

---

## Install

**Homebrew (macOS / Linux):**

```bash
brew install AndreasSteinerPF/tm/tm
```

**Scoop (Windows):**

```powershell
scoop bucket add tm https://github.com/AndreasSteinerPF/tm-scoop
scoop install tm
```

**Shell installer (POSIX):**

```bash
curl -fsSL https://raw.githubusercontent.com/AndreasSteinerPF/team-memory/main/install.sh | sh
```

Drops the latest `tm` binary into `~/.local/bin` (override with `TM_INSTALL_DIR`). Verifies the SHA-256 against the release's `checksums.txt` before installing.

**From source (Go 1.26+):**

```bash
go install github.com/AndreasSteinerPF/team-memory/cmd/tm@latest
```

Or download a prebuilt archive from [Releases](https://github.com/AndreasSteinerPF/team-memory/releases).

---

## Quickstart

### 1. Initialize in your repo

```bash
cd your-repo
tm init
```

Creates an orphan branch `teammemory`, a local SQLite index under `.git/tm/`, and (when `.claude/` exists) installs the Claude Code hooks: `PreToolUse` check on edits *and Bash commands*, `SessionStart` briefing, and the near-moment nudge engine (`PostToolUse` signal + `Stop` nudge + `UserPromptSubmit` marker).

### 2. Propose a memory

```bash
tm propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --guidance "Run downgrade tests before modifying billing migrations." \
  --scope "billing/migrations/**" \
  --session "$CLAUDE_SESSION_ID"
```

Memories can be scoped to **commands** as well as paths. Use `--scope-command` for a lesson that bites when a command runs, not when a file is edited:

```bash
tm propose constraint \
  --title "pytest needs DATABASE_URL set" \
  --guidance "Export DATABASE_URL before running the test suite." \
  --scope-command "pytest *" \
  --session "$CLAUDE_SESSION_ID"
```

Output:

```
01J8X4QZ7M9FKE2V3R5T8WYBCD
status: provisional   risk: high   confidence: low   enforcement: hint
reason: awaiting independent confirmation
```

### 3. Check a path or command before acting

```bash
tm check-action --path billing/migrations/new_migration.sql
tm check-action --command "pytest -q tests/"
```

### 4. Export to your context files

```bash
tm export --format agents --out AGENTS.md
tm export --format claude --out CLAUDE.md
tm export --format json            # prints JSON to stdout
```

---

## Features

- **Stop repeating known-bad approaches.** An agent records a failed approach with evidence; the next agent that opens the same area is warned before it tries again.
- **Block bad moves at edit and command time.** Validated memories promoted to `requirement` make the `PreToolUse` hook deny matching edits and Bash commands until acknowledged — tribal knowledge becomes a guardrail no agent can skip.
- **Memory earns trust through evidence.** A memory stays provisional until another agent (different session, different branch) independently confirms it. No single agent can unilaterally create a binding rule; only humans can promote to `requirement`.
- **One ledger, every coding agent.** Claude Code, Codex, Cursor, Continue, Copilot, and Gemini CLI all share the same memories — same rules, same enforcement, same Git-backed audit trail.
- **Audit every change as plain Git.** The ledger is an append-only orphan branch; `git log teammemory` shows who proposed what, who confirmed it, and when it became binding.

---

## How it compares

| Class | Examples | What they do | TeamMemory's difference |
|---|---|---|---|
| Auto-capture memory | claude-mem, Mem0/OpenMemory, Cipher | Observe sessions, compress, accumulate | Evidence-validated lifecycle: memories earn trust through independent confirmation; contradictions weaken them |
| Hosted team memory | Cloudflare Agent Memory, Supermemory | Shared memory via a hosted API | Git-native and local-first: the ledger lives in your repo, auditable via `git log`, no SaaS dependency |
| Platform-native memory | Claude managed memory, Cursor memories | Per-platform memory stores | Cross-agent and team-scoped: one ledger serves Claude Code, Codex, Cursor, Continue |
| Static context files | `CLAUDE.md`, `AGENTS.md`, `.cursor/rules` | Hand-maintained instructions | Evolves through work; context files become generated projections |

In short: **evidence-validated, Git-native, governed team memory with a deterministic enforcement point.**

---

## Demo

The flagship demo walks the full lifecycle — a provisional memory becoming an enforced requirement through ordinary agent work, across two branches and three sessions. Run the whole thing in one command:

```bash
bash demo/run.sh
```

What it does, step by step (illustrative — `demo/run.sh` runs the real thing end to end):

```bash
# Agent A, on feature/invoice-state, burns a session on a rollback that fails.
# It records the lesson with evidence.
tm propose failed_attempt \
  --title   "Billing migrations require downgrade-path tests" \
  --summary "Rollback failed when invoice_state migration lacked a downgrade path." \
  --guidance "Before modifying billing migrations, check rollback behavior and add downgrade-path tests." \
  --scope   "billing/migrations/**" \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor  "billing/migrations/2026_add_invoice_state.sql@HEAD" \
  --session session_a
# → provisional   risk: high   confidence: low   enforcement: hint

ID=01J8X4QZ7M9FKE2V3R5T8WYBCD   # from the output

# Agent B, on a different branch and session, hits the same wall and confirms.
# Independent confirmation auto-activates the memory.
tm observe $ID confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --session session_b
# → status: active   confidence: medium   enforcement: warning

# A human escalates it to a hard requirement.
tm approve $ID --enforcement requirement --confidence high

# Agent C tries to edit a billing migration. The PreToolUse hook BLOCKS the edit.
# Agent C runs the downgrade tests, acknowledges the requirement, and retries — now it proceeds.
tm ack $ID --session session_c --note "downgrade tests pass"
```

Every step is auditable as ordinary Git history:

```bash
git log teammemory -- memories/ observations/
```

---

## Memory lifecycle

```
propose → provisional
  + independent confirmation (different session, different branch)
    → active (warning enforcement)
  + human approve --enforcement requirement
    → active (requirement enforcement) — hook blocks matching edits and commands until acked
  + observe contradict (from any session)
    → contested (confidence reduced)
  + observe mark_stale
    → stale
  + observe mark_duplicate --canonical-id <other>
    → duplicate (auto-effect; hidden from default retrieval)
  + observe supersede --supersedes <obsolete>   (filed on the NEW canonical)
    + substantiation: independent confirm or human approve on the canonical
    → obsolete memory becomes superseded
```

`successful_pattern` is the one type that overrides its risk tier: even though it is low-risk, it stays `provisional` until at least one independent session confirms it (or a maintainer approves it). The intent is to keep unilateral pattern proposals from auto-activating without evidence.

- **Status:** `provisional` → `active` → `contested` / `stale` / `duplicate` / `superseded` / `rejected`.
- **Enforcement:** `hint` → `recommendation` → `warning` → `requirement`. Only a human can set `requirement`.
- **Risk** (`low` / `medium` / `high` / `critical`) is computed deterministically from `policy.yaml` — never from agent self-assessment. High-risk paths (e.g. `**/migrations/**`) escalate automatically, as do broad command scopes (a bare-binary pattern like `pytest *`).

Status, risk, confidence, and enforcement are always *derived* from the ledger and policy. They are never stored as mutable fields an agent could set directly.

---

## Memory types

Six typed envelopes, each with a free-form summary and guidance:

| Type | When to use it |
|---|---|
| `failed_attempt` | An approach that was tried and failed, with evidence. |
| `constraint` | A rule on how work must be done here. `--origin team` (internal convention) or `--origin external` (third-party/API contract). |
| `fragile_area` | A path where changes frequently break non-obvious things. |
| `stale_doc` | A document that is outdated or misleading — ideally pointing to what supersedes it. |
| `decision` | A decision that changes future work and isn't written down anywhere else. |
| `successful_pattern` | A repeatedly-applied refactor, approach, or workflow with a measurable outcome. Carries a type-specific activation gate — stays `provisional` until one independent session confirms it. A single function that worked once is NOT a pattern. |

---

## Commands

```
tm init          create orphan branch, default policy, local index; install Claude Code hooks
tm sync          fetch + union-merge + push the teammemory branch
tm check-action  query memory for an action (--hook mode for the PreToolUse hook)
tm signal        record nudge signals (PostToolUse) or a prompt marker (--prompt) for the nudge engine
tm nudge         emit at most one propose/observe nudge at turn end (--hook mode for the Stop hook)
tm brief         session-start briefing for agent hooks (live counts + instructions)
tm propose       create a memory record
tm observe       add an observation (confirm / contradict / adjust_scope / mark_stale /
                 mark_duplicate --canonical-id / supersede --supersedes)
tm ack           session-scoped requirement acknowledgment (local-only, never committed)
tm approve       activate a memory; set enforcement and confidence (human action)
tm reject        kill a memory permanently (human action)
tm remote        show / set / unset the ledger remote (separate-remote mode)
tm list          list memories (--stale, --contested, --stale-candidates,
                 --duplicate, --superseded, --pending-supersede)
tm show          full detail: envelope, observations, derived state
tm search        lexical search over titles, summaries, guidance
tm export        generate AGENTS.md / CLAUDE.md / .cursor/rules blocks or JSON
tm status        ledger overview, items needing human attention, sync state
tm doctor        diagnose setup: ledger branch, index, hooks, MCP, remote
```

Run any command with `--help` for its full flag set. Memories are scoped with `--scope` (path globs) and/or `--scope-command` (command patterns such as `pytest *`); `tm check-action` takes `--path` and/or `--command`. `tm observe mark_duplicate` requires `--canonical-id <other-memory>` (filed on the duplicate, names the kept memory); `tm observe supersede` requires `--supersedes <obsolete-memory>` (filed on the NEW canonical, names the obsolete one). `tm propose` and `tm observe` also accept `--actor`, `--session`, `--ctx-branch`, `--ctx-path`, and `--ctx-command` to attribute records and record code context.

---

## Claude Code integration

### Edit-time hook

`tm init` installs the hook automatically when `.claude/` is present. To install manually, add to `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit|Bash",
        "hooks": [{ "type": "command", "command": "tm check-action --hook" }]
      }
    ]
  }
}
```

The hook:

- Reads the tool input (the file path for edits, the command for Bash) and the current session ID from stdin.
- Queries the local index — no network, no subprocess beyond the binary.
- Returns `deny` for matching unacknowledged `requirement` memories, `additionalContext` for warnings.
- Matches Bash commands against memory command patterns (token-aware, leading-subcommand; flags aren't matched). Command matching is best-effort across shell composition (pipes, `&&`).
- Completes in under 100 ms on a 1,000-memory ledger.

### MCP server

`tm init` registers the `teammemory` MCP server in the repo-root `.mcp.json` automatically (merge-safe — existing servers are preserved). The resulting entry:

```json
{
  "mcpServers": {
    "teammemory": { "command": "tm", "args": ["mcp"] }
  }
}
```

MCP tools: `tm_propose`, `tm_observe`, `tm_check_action`, `tm_search`, `tm_status`.

### Session-start briefing

`tm brief` emits a short briefing — live ledger counts plus standing instructions for `tm_propose` / `tm_observe` / `tm_check_action` — designed to be injected into agent context at session start. `tm init` installs it automatically for Claude Code as a `SessionStart` hook. In a repo without an initialized ledger it prints nothing and exits 0, so the hook is always safe to install.

### Near-moment nudges

The session briefing tells an agent *when* to remember; the nudge engine catches the moments it would otherwise miss. `tm init` installs three more hooks for Claude Code:

- **`PostToolUse` → `tm signal --hook`** records the raw events — command outcomes and edits — into a per-session journal under `.git/tm/nudge`. It is silent, never blocks, and advances a within-session turn counter.
- **`Stop` → `tm nudge --hook`** runs at turn end. It reads the journal, detects the memory-worthy patterns (a test going fail→pass, a reverted change, repeated edit churn on one path, a surfaced-but-unobserved memory, anchor drift), and emits **at most one** proposing/observing nudge, injected as additional context (never a forced turn). An anti-spam policy bounds it: max 3 per session, a cooldown between nudges, suppress-if-already-acted, and `observe` outranks `propose`.
- **`UserPromptSubmit` → `tm signal --hook --prompt`** records a prompt marker so the engine can detect an edit→prompt→re-edit on the same path (a signal that the user redirected the agent there).

The nudge journal is local state under `.git/tm/nudge` — like acks, it is never committed to the ledger. The verbs themselves stay voluntary; the engine only raises the highest-value moments to a pointed prompt.

---

## Other agents

Every agent reads the same ledger; what differs is the delivery guarantee:

| Agent | Hook enforcement (edit + command) | Session briefing | Near-moment nudges | Voluntary verbs (MCP) | Static fallback |
|---|:---:|:---:|:---:|:---:|:---:|
| Claude Code | ✅ | ✅ | ✅ † | ✅ | ✅ |
| Codex CLI | ✅ | ✅ | ✅ † | ✅ | ✅ |
| Continue CLI | ✅ | ✅ | ✅ | ✅ | ✅ |
| Copilot CLI | ✅ | ✅ | ✅ | ✅ | ✅ |
| Cursor | ✅ | ✅ | ✅ | ✅ | ✅ (`.cursor/rules`) |
| Gemini CLI | ✅ | ✅ | ✅ | ✅ | ✅ |
| Other MCP / hook-less | — | — | — | ✅ | ✅ (only path) |

> † The **fail→fix→pass** nudge detector does not fire on Claude Code or Codex: both run `PostToolUse` only on tool *success*, so a failed command is never observed. Every other nudge detector — reverted change, repeated edit churn on one path, user redirected mid-edit, surfaced-but-unobserved memory, anchor drift — works on both. (See `prd.md §10.6`.)

Every hook-capable agent enforces `requirement` memories deterministically: `tm init --harness <name>` installs a pre-tool hook — Claude Code's `PreToolUse` and the equivalent on Codex, Copilot, Cursor, and Gemini (Continue reuses Claude Code's hook schema) — that **blocks** a matching edit or Bash command until it's acked, rendering the deny in each harness's native hook format. Only genuinely hook-less agents fall back to a voluntary `check_action` over MCP — same knowledge, but a voluntary call rather than a guaranteed one.

The near-moment nudge engine is harness-neutral: a thin adapter maps each tool's post-tool, prompt, and turn-end events onto the same `tm signal` / `tm nudge` verbs, so Codex, Copilot, Cursor, and Gemini get the same propose/observe nudges as Claude Code. `tm init --harness <name>` wires the per-tool hooks (event names differ; the engine and anti-spam budget do not).

The MCP server works with any MCP-compatible agent. For agents without MCP, `tm export` generates instruction blocks that are clearly marked and never the source of truth — the ledger is.

```bash
# Add to your context file once; re-run when memories change.
tm export --format agents --out AGENTS.md
```

`tm brief` supports per-tool output formats for session-start hooks (snippets abridged — consult each tool's hooks reference):

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

---

## Sync (team use)

**Sync is automatic — you rarely run `tm sync` by hand.** Memories flow in both directions in the background:

- **Outgoing:** `tm propose` and `tm observe` push the ledger branch in the background (detached, best-effort). If you're offline or the push is rejected, the record stays local and reconciles later — the command never blocks or fails on the network. Stable rejections (e.g. branch protection) are classified and surfaced through `tm status` and `tm doctor` so you find out without watching git logs — see [Branch protection / separate-remote mode](#branch-protection--separate-remote-mode).
- **Incoming:** `tm check-action` (including the `PreToolUse` hook) triggers a non-blocking background fetch when the last fetch is older than `sync.auto_fetch_after` (default 5 minutes). Teammates' memories arrive as you work, without anyone running a command.

`tm sync` is the manual **reconciliation fallback** — run it when you were offline and want to flush queued records, when the remote diverged (a background push was rejected), or when you want an immediate refresh instead of waiting for the next opportunistic fetch:

```bash
tm sync

# With a separate remote for the ledger branch:
tm sync --remote git@github.com:org/repo-memory.git
```

`tm init --remote <name-or-url>` stores a separate ledger remote as `git config tm.remote`, validates it (`ls-remote`), and seeds the orphan `teammemory` ref with a best-effort push so teammates can fetch immediately. `tm sync`, background fetch, and background push all honor the stored value. Pass `--no-push` for an offline / CI bootstrap.

### Branch protection / separate-remote mode

If your code remote protects all branches (preventing the orphan `teammemory`
branch from being pushed), point the ledger at a separate remote:

    git remote add memory git@github.com:acme/repo-memory.git
    tm remote set memory

`tm sync`, the background push, and the opportunistic fetch all honor it.
`tm doctor` and `tm status` warn if recent pushes have been rejected
(usually a sign that branch protection is still in the way).

`tm remote show` prints the current ledger remote; `tm remote unset` reverts
to `origin`.

Sync uses **union-merge**: because each record is an append-only ULID-named file, concurrent proposals from different clones never conflict.

---

## Policy

`tm init` writes `policy.yaml` to the ledger branch. Two knobs do most of the work: `escalators.sensitive_paths` (which paths get a minimum risk floor) and `activation.tiers` (how much independent confirmation each risk tier needs before it auto-activates, and how far enforcement can rise without a human). Excerpt of the defaults:

```yaml
escalators:
  broad_scope_bump: true
  sensitive_paths:
    - glob: '**/migrations/**'
      min_risk: high
    - glob: '**/auth/**'
      min_risk: critical
    - glob: .github/workflows/**
      min_risk: critical
activation:
  independence: different_session
  tiers:
    low:                                  # auto-activates immediately
      auto: immediate
      max_auto_enforcement: recommendation
    high:                                 # one independent confirm activates it
      auto: independent_confirm
      max_auto_enforcement: warning
    critical:                             # two independent confirms activate it
      auto: independent_confirm
      min_independent_confirms: 2
      max_auto_enforcement: warning
```

`critical` memories need two independent confirmations to auto-activate — more evidence than any other tier — and no tier can reach `requirement` without `tm approve`, so agents alone can never create a binding rule.

---

## Context cost

TeamMemory is deliberately stingy with your agent's context window: every injection point is policy-capped, retrieval is precision-first so most tool calls add nothing, and the worst-case session ceiling sits well under what a modern context window will notice.

Measured numbers (≈ tokens, rounded; varies by model tokenizer):

- **At session start, once:**
  - MCP tool descriptions in the tool list: **~1,000 tokens**
  - `tm brief` SessionStart injection (live counts + standing instructions): **~250 tokens**
- **Per matching tool call** (most calls don't match a memory and add nothing):
  - 1 match → **~100 tokens**
  - Cap saturated at 5 active + 2 provisional → **~400 tokens**
- **At turn end:** at most one near-moment nudge of **~150 tokens**, capped at **3 per session**.

A busy session typically accumulates **~2,000–4,000 tokens** of tm-injected content end to end — under 4% of a 100K-token context window, and re-read from the model's prompt cache (not retokenized) on subsequent turns. Compare to static context files that re-ship full instructions every turn, or auto-capture memory tools that grow unboundedly.

Every cap is policy-driven (`retrieval.max_results`, `retrieval.max_provisional`, `nudge.max_per_session` in `policy.yaml`) and `nudge.enabled: false` turns the near-moment engine off entirely if you'd rather only the voluntary verbs fire.

---

## How it works

- **Ledger:** an orphan Git branch `teammemory` stores YAML memory and observation records as ULID-named files. No code-branch pollution.
- **Index:** a local SQLite database under `.git/tm/` materializes derived state and supports full-text search. Rebuilt automatically; throwaway.
- **Derived state:** status, risk, confidence, and enforcement are computed deterministically from the ledger and policy — never stored, never guessable.
- **Sync:** union-merge of the orphan branch. Concurrent proposals never conflict because each record is an append-only ULID file.
- **Hook:** the `PreToolUse` hook (on edits and Bash commands) reads the index (no network, no ledger-branch checkout) and completes in under 100 ms.
- **Nudge engine:** `PostToolUse`/`UserPromptSubmit` record raw events to a per-session journal under `.git/tm/nudge`; the `Stop` hook detects the memory-worthy patterns and emits at most one bounded propose/observe nudge. Detection is pure and the journal is local-only — never a ledger record.

---

## Roadmap

See [`prd.md §17`](prd.md#17-roadmap) for the full roadmap. **Phase 1** (MVP) and **Phase 2** (cross-harness adapters, separate-remote UX, package-manager distribution) are shipped. Coming up:

- **Phase 3 — GitHub workflow.** A GitHub Action that posts relevant memories on a PR's changed paths/commands, plus a static HTML timeline report of the ledger's history.
- **Phase 4 — Retrieval depth.** Symbol anchors, error-signature matching, content-hash drift detection, and embeddings-based semantic ranking.
- **Phase 5 — Harness breadth.** Adapters for OpenCode, Pi, and other coding agents as they gain a comparable hook surface.
- **Phase 6 — Governance depth.** Signed records, multi-human approval, policy templates, expiry workflows.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, PR conventions, and the cross-harness test framework. By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md). For security issues, see [SECURITY.md](SECURITY.md).

---

## License

MIT — see [LICENSE](LICENSE).
