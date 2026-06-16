# Auto-register MCP for every harness

**Date:** 2026-06-16
**Status:** Approved (design)

## Problem

`tm init` registers the `teammemory` MCP server inconsistently across harnesses:

- **Cursor** (`.cursor/mcp.json`) and **Gemini** (`.gemini/settings.json`) auto-write MCP
  config, because their config is repo-local.
- **Claude** uses a repo-local `.mcp.json` too, yet `tm init` only *prints* the snippet as a
  manual "Next steps" instruction (`init.go` `printSetup`).
- **Codex** (`~/.codex/config.toml`) and **Copilot** (`~/.copilot/mcp-config.json`) only *print*
  guidance, because their MCP config lives in the user's home directory and `tm init` has so far
  never mutated `$HOME`.

The result: Claude — the flagship harness — is the one that makes you hand-edit config, and three
of five harnesses require manual MCP setup. `tm doctor` flags MCP as a warning with a "add
manually" remediation.

## Goal

`tm init` registers the `teammemory` MCP server automatically for **all five** harnesses,
**merging into existing config rather than clobbering it**. This deliberately extends `tm init`
to write into `$HOME` for Codex and Copilot, since that is where those harnesses read MCP config.

## Per-harness target & entry

| Harness | File | Format | Server entry |
|---|---|---|---|
| Claude  | `<repo>/.mcp.json`               | JSON | `{"command":"tm","args":["mcp"]}` |
| Copilot | `~/.copilot/mcp-config.json`     | JSON | `{"type":"local","command":"tm","args":["mcp"]}` |
| Codex   | `~/.codex/config.toml`           | TOML | `[mcp_servers.teammemory]` block (`command = "tm"`, `args = ["mcp"]`) |
| Cursor  | `<repo>/.cursor/mcp.json`        | JSON | `{"type":"stdio","command":"tm","args":["mcp"]}` (entry unchanged; write path retrofitted) |
| Gemini  | `<repo>/.gemini/settings.json`   | JSON | unchanged (already auto) |

## Components

### 1. `ensureMCPServerJSON(path string, entry map[string]any) (bool, error)`

A shared, merge-safe JSON helper, mirroring the pattern of `installClaudeCodeHooks` in
`internal/cli/plugin.go`:

1. Read + unmarshal the file into `map[string]any` if it exists (empty map if absent).
2. Ensure a `mcpServers` sub-map exists.
3. If `mcpServers["teammemory"]` is already present → no-op, return `(false, nil)`.
4. Otherwise insert `entry`, marshal with indent, write (creating parent dir if needed),
   return `(true, nil)`.

Preserves every other server under `mcpServers` and every other top-level key.

**Consumers:** the Claude path (`.mcp.json`) and the Copilot path (`~/.copilot/mcp-config.json`).
`installCursor` is **retrofitted** to use this helper instead of overwriting the whole
`.cursor/mcp.json` — the current overwrite silently clobbers a user's other MCP servers, a latent
bug, and the fix is the same helper. Gemini's combined `settings.json` (hooks + MCP in one file)
is **out of scope** — a separate concern from single-purpose MCP files.

### 2. Codex TOML — append-if-absent

No TOML library exists in `go.mod`, and this design does **not** add one, nor shell out to
`codex mcp add` (which would couple to the `codex` binary being installed). Instead:

- Resolve `~/.codex/config.toml`.
- If the file already contains the table header `[mcp_servers.teammemory]` → no-op (idempotent).
- Otherwise ensure the existing content ends with a newline, then append:

  ```toml

  [mcp_servers.teammemory]
  command = "tm"
  args = ["mcp"]
  ```

Appending a table at end-of-file is valid TOML (a table continues until the next header or EOF).
Create `~/.codex/` if missing.

### 3. `$HOME` handling

`installCodex` and `installCopilot` gain a `homeDir string` parameter. `init` resolves it once via
`os.UserHomeDir()` and passes it down. This keeps both functions unit-testable against a temp home
and avoids scattering `os.UserHomeDir()` calls through the install layer. Each creates its
`~/.codex` / `~/.copilot` directory if missing.

### 4. Output messages

Replace the printed-guidance lines ("run `codex mcp add`…", "add to `~/.copilot/...`") with what
actually happened:

- `Registered teammemory MCP server in <path>.` when newly written.
- `teammemory MCP server already registered in <path>.` when it was already present.

## Error handling

Merge failures (unreadable file, invalid existing JSON/TOML) surface as errors from `tm init`,
the same as the hook installer does today. A pre-existing `teammemory` entry is a clean no-op,
never an error.

## Documentation (same commit, per AGENTS.md)

- **prd.md §10.6** packaging paragraph: Codex/Copilot change from "printed guidance" to "merged
  into `~/.codex/config.toml` / `~/.copilot/mcp-config.json`"; note Claude writes `.mcp.json`.
- **internal/cli/doctor.go**: the MCP-missing remediation changes from "add manually" to
  "run `tm init`".
- **README.md**: update the MCP sections (Claude integration, Other agents) to reflect automatic
  registration.

## Testing (TDD)

- `ensureMCPServerJSON`: creates file when absent; inserts preserving existing servers and other
  top-level keys; idempotent (second call returns `false`, no duplicate).
- `installCodex` against a temp `homeDir`: creates `~/.codex`, writes the table, preserves
  pre-existing content, idempotent.
- `installCopilot` against a temp `homeDir`: creates `~/.copilot`, writes/merges the entry,
  preserves existing servers, idempotent.
- Default `tm init` writes repo `.mcp.json` with the `teammemory` entry.
- Extend the existing packaging-tier harness test (`e2e/harness`) to assert each `--harness`
  writes/merges its MCP target.

## Explicitly out of scope

- Adding a TOML dependency.
- Shelling out to `codex mcp add`.
- Reworking Gemini's `settings.json`.
- Any change to hook installation.
