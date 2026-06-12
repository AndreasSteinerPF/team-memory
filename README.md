# TeamMemory

**Agents propose. Agents observe. Teams remember.**

TeamMemory is a Git-backed collaborative memory ledger for coding agents. Agents propose repo-scoped memories during normal work; other agents confirm, contradict, or refine them when they encounter related code; validated memories reach future agents deterministically through an edit-time hook — not just a voluntary tool call.

It is not a general memory system, not an agent framework. It is a focused system for preserving **future-action-relevant project judgment**: failed attempts, hidden constraints, fragile areas, stale docs, and undocumented decisions that should influence future agent behavior.

---

## Quickstart (under 10 minutes)

### 1. Install

```bash
go install github.com/AndreasSteinerPF/team-memory/cmd/tm@latest
```

Or download a binary from [Releases](https://github.com/AndreasSteinerPF/team-memory/releases).

### 2. Initialize in your repo

```bash
cd your-repo
tm init
```

This creates an orphan branch `teammemory` and a local SQLite index under `.git/tm/`. If `.claude/` exists, it also installs the PreToolUse hook in `.claude/settings.json` automatically.

### 3. Propose a memory

```bash
tm propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --guidance "Run downgrade tests before modifying billing migrations." \
  --scope "billing/migrations/**" \
  --session "$CLAUDE_SESSION_ID"
```

Output:

```
Created memory 01J8X4QZ7M9FKE2V3R5T8WYBCD
  type: failed_attempt   risk: high   status: provisional   confidence: low
  scope: billing/migrations/**
```

### 4. Check a path before editing

```bash
tm check-action --path billing/migrations/new_migration.sql
```

### 5. Export to AGENTS.md / CLAUDE.md

```bash
tm export --format agents --out AGENTS.md
tm export --format claude --out CLAUDE.md
tm export --format json
```

---

## The Flagship Demo

**Ambient memory validation across branches** — shows a provisional memory becoming a requirement block through normal agent work.

```bash
# Agent A (session s1) proposes after a rollback failure.
tm propose failed_attempt \
  --title "Billing migrations require downgrade-path tests" \
  --scope "billing/migrations/**" \
  --summary "Rollback failed when invoice_state migration lacked downgrade path." \
  --evidence "test_failure:logs/rollback_failure.log" \
  --anchor "billing/migrations/2026_add_invoice_state.sql@HEAD" \
  --session s1

ID=01J8X4QZ7M9FKE2V3R5T8WYBCD   # from output

# Agent B (session s2) independently confirms — auto-activates the memory.
tm observe $ID confirm \
  --summary "Same rollback failure reproduced on revenue-reporting branch." \
  --session s2
# → status: active, confidence: medium, enforcement: warning

# Human escalates to a hard requirement.
tm approve $ID --enforcement requirement --confidence high

# Agent C (session s3) attempts to edit a billing migration.
# The PreToolUse hook fires and blocks the edit until C acks the requirement.
tm ack $ID --session s3
# Now the edit proceeds.
```

Every step of the ledger is auditable:

```bash
git log teammemory -- memories/ observations/
```

---

## Memory Lifecycle

```
propose → provisional
  + independent confirmation (different session, different branch)
    → active (warning enforcement)
  + human approve --enforcement requirement
    → active (requirement enforcement) — hook blocks edits until acked
  + observe contradict (from any session)
    → contested (confidence reduced)
  + observe mark_stale
    → stale
```

Risk (`low` / `medium` / `high`) is computed deterministically from `policy.yaml` — never from agent self-assessment. High-risk paths (e.g. `**/migrations/**`) automatically escalate.

---

## Commands

```
tm init          create orphan branch, default policy, local index; install Claude Code hook
tm sync          fetch + union-merge + push the teammemory branch
tm check-action  query memory for an action (--hook mode for the PreToolUse hook)
tm propose       create a memory record
tm observe       add an observation (confirm / contradict / adjust_scope / mark_stale)
tm ack           session-scoped requirement acknowledgment (local-only, never committed)
tm approve       activate a memory; set enforcement and confidence (human action)
tm reject        kill a memory permanently (human action)
tm list          list memories (--stale, --contested, --stale-candidates)
tm show          full detail: envelope, observations, derived state
tm search        lexical search over titles, summaries, guidance
tm export        generate AGENTS.md / CLAUDE.md / .cursor/rules blocks or JSON
tm status        ledger overview, items needing human attention, sync state
```

---

## Claude Code Integration

### Hook (edit-time enforcement)

`tm init` installs the hook automatically when `.claude/` is present. To install manually:

Add to `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [{ "type": "command", "command": "tm check-action --hook" }]
      }
    ]
  }
}
```

The hook:
- Reads the tool input path and the current session ID from stdin
- Queries the local index (no network, no subprocess beyond the binary)
- Returns `deny` for unacknowledged `requirement` memories, `additionalContext` for warnings
- Completes in under 100ms on a 1,000-memory ledger

### MCP server

Add to `.mcp.json`:

```json
{
  "mcpServers": {
    "teammemory": { "command": "tm", "args": ["mcp"] }
  }
}
```

MCP tools: `tm_propose`, `tm_observe`, `tm_check_action`, `tm_search`, `tm_status`.

---

## Other Agents (Cursor, Codex, Continue)

The MCP server works with any MCP-compatible agent. For agents without MCP, `tm export` generates instruction blocks:

```bash
# Add to your context file once; re-run when memories change.
tm export --format agents --out AGENTS.md
```

The generated block is clearly marked and never the source of truth — the ledger is.

---

## Sync (team use)

```bash
# After teammates push new memories:
tm sync

# With a separate remote for the ledger branch:
tm sync --remote git@github.com:org/repo-memory.git
```

Sync uses union-merge: concurrent proposals from different clones never conflict.

---

## Policy

`tm init` writes `policy.yaml` to the ledger branch. Edit it to tune risk escalation:

```yaml
sensitive_path_patterns:
  - "**/migrations/**"
  - "**/secrets/**"
auto_activate:
  min_independent_confirms: 1
  risk_tiers:
    high:
      min_confirms: 1
    medium:
      min_confirms: 2
    low:
      min_confirms: 3
```

---

## How It Works

- **Ledger:** an orphan Git branch `teammemory` stores YAML memory and observation records as ULID-named files. No code-branch pollution.
- **Index:** a local SQLite database under `.git/tm/` materializes derived state and supports FTS. Rebuilt automatically; throwaway.
- **Derived state:** status, risk, confidence, and enforcement are computed deterministically from the ledger and policy — never stored, never guessable.
- **Sync:** union-merge of the orphan branch. Concurrent proposals never conflict because each record is an append-only ULID file.
- **Hook:** the PreToolUse hook reads the index (no network, no ledger branch checkout) and completes in <100ms.

---

## Contributing

1. `go test ./... -count=1` must be green before any PR.
2. The flagship demo (`TestFlagshipDemo`) and trap-repo benchmark (`TestTrapRepoBenchmark`) are the acceptance tests — keep them passing.
3. Derived state (`internal/derive`) is the single most-depended-on package; any change there requires updated golden fixtures.

---

## License

MIT
