# Cross-harness configuration

TeamMemory works the same across all hook-capable coding agents — same ledger, same memories, same enforcement. `tm init --harness <name>` writes the right config for each. This page covers the per-harness specifics; the README's [Other agents](../README.md#other-agents) section has the high-level capability matrix.

## How enforcement works across harnesses

Every hook-capable agent enforces `requirement` memories deterministically: `tm init --harness <name>` installs a pre-tool hook — Claude Code's `PreToolUse` and the equivalent on Codex, Copilot, Cursor, and Gemini (Continue reuses Claude Code's hook schema) — that **blocks** a matching edit or Bash command until it's acked, rendering the deny in each harness's native hook format. Only genuinely hook-less agents fall back to a voluntary `check_action` over MCP — same knowledge, but a voluntary call rather than a guaranteed one.

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
