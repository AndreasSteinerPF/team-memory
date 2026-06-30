# Cross-harness configuration

> This document is an explanatory projection of the authoritative
> [`prd.md`](../prd.md). If they differ, `prd.md` governs intended behavior;
> code and tests determine what is implemented.

TeamMemory uses the same ledger, memories, policy, and shared engine across
supported coding agents. Delivery and enforcement capabilities vary with each
agent's hook API. `tm init --harness <name>` writes the relevant configuration.
This page covers those differences; the README's
[Other agents](../README.md#other-agents) section has the high-level matrix.
This document is an explanatory projection of `prd.md §10.6`.

## How enforcement works across harnesses

Claude Code, Codex, Copilot, and Gemini install pre-tool enforcement for the
actions their hook APIs expose. Cursor is the exception: its pre-tool surface is
`beforeShellExecution`, so command-scoped requirements block before execution,
while path-scoped memories can surface only after `afterFileEdit` and cannot
prevent that edit (`prd.md §10.6`). Continue can reuse Claude Code's hook schema
but has no dedicated `tm init --harness continue` installer. Genuinely hook-less
agents fall back to voluntary `check_action` over MCP.

The near-moment nudge engine is harness-neutral: a thin adapter maps each tool's post-tool, prompt, and turn-end events onto the same `tm signal` / `tm nudge` verbs, so Codex, Copilot, Cursor, and Gemini get the same propose/observe nudges as Claude Code. `tm init --harness <name>` wires the per-tool hooks (event names differ; the engine and anti-spam budget do not).

## Static fallback for non-hook agents

The MCP server works with any MCP-compatible agent. For agents without MCP, `tm export` generates instruction blocks that are clearly marked and never the source of truth — the ledger is.

```bash
# Add to your context file once; re-run when memories change.
tm export --format agents --out AGENTS.md
```

## Per-tool session-start briefing

`tm brief` supports per-tool output formats for session-start hooks. The snippets below are abridged — consult each tool's hooks reference for the canonical schema.

### Codex CLI (`.codex/config.toml`; requires a trusted workspace)

```toml
[[hooks.SessionStart]]
command = ["tm", "brief"]
```

### Copilot CLI (`.github/hooks/teammemory.json`)

```json
{ "version": 1, "hooks": { "sessionStart": [{ "type": "command", "command": "tm brief --format copilot" }] } }
```

### Cursor (`hooks.json`)

```json
{ "version": 1, "hooks": { "sessionStart": [{ "command": "tm brief --format cursor" }] } }
```

### Gemini CLI (`settings.json`)

```json
{ "hooks": { "SessionStart": [{ "type": "command", "command": "tm brief --format gemini" }] } }
```

### Continue CLI

Hook schemas are Claude Code-compatible — use the same entry `tm init` writes for Claude Code.

## Live verification

For per-harness wire-format findings and live behavior tests (the actual payloads each CLI emits, the trust gates, the BOM workarounds), see [docs/verification/cross-harness.md](verification/cross-harness.md).
