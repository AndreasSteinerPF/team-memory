[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html

# Changelog

All notable changes to TeamMemory are documented here. The format is based on
[Keep a Changelog], and this project adheres to [Semantic Versioning].

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

[0.1.0]: https://github.com/AndreasSteinerPF/team-memory/releases/tag/v0.1.0
