# Cursor live tier — notes

The headless `cursor-agent` CLI (installed as `agent`) fires `.cursor/hooks.json`
hooks (verified, cursor 2026.06.12). Driver: `agent -p --force --trust "<prompt>"`
(prompt is a trailing positional; `-p`=headless, `--force`=auto-approve,
`--trust`=trust workspace). The binary may live at
`%LOCALAPPDATA%\cursor-agent\agent.cmd` and not be on a non-interactive PATH —
ensure `agent` is on PATH for the live tier.

## Headless limitations (verified)

- The headless CLI does NOT fire `stop` or `beforeSubmitPrompt`. So `tm nudge`
  (Stop) and prompt signals are exercised only in the Cursor IDE or via replayed
  fixtures. `fail_pass_nudge/stop.json` and any prompt fixture stay `authored`.
- A failed shell command's `postToolUseFailure` nests the command at
  `tool_input.command` (not top-level) — handled by `internal/harness/cursor.go`.
- `afterShellExecution` has no exit code and fires for pass and fail alike; the
  failure signal is the separate `postToolUseFailure` event.

So `task capture:cursor` captures `cmd-fail`/`edit`/`edit-scoped`; `cmd-pass` and
`stop` stay authored (see Task 7 in the plan).
