# Cross-harness live-payload verification recipes

## Purpose

TeamMemory's Codex, Copilot, Cursor, and Gemini CLI adapters were built from
published specs and source inspection, but several assumptions cannot be
validated by unit tests: they depend on wire shapes that only a live harness
emits. The checks below require the respective CLI to be installed and
configured in a real project. Once a verifier runs each recipe and confirms the
payload matches what the adapter expects, they can tick the corresponding
`VERIFY (spec §10)` annotation in the adapter source file, and update the
status note in `prd.md §10.6`.

---

## Method: the echo hook

The general technique is simple:

1. Temporarily register a hook whose only job is to dump its stdin to a file.
2. Trigger a known action in the harness (run a command, edit a file, cause a
   failure).
3. Read the captured JSON file and grep for the field of interest.

**Unix echo command** (use this as the hook `command` value):

```sh
sh -c 'cat > /tmp/tm-hook-$(date +%s).json'
```

**Windows/PowerShell equivalent** (substitute where the hook config accepts a
shell command; Copilot CLI uses `bash` on Windows via WSL or Git Bash):

```powershell
# In PowerShell, write stdin to a timestamped file:
$f = "C:\Temp\tm-hook-$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds()).json"
$input | Set-Content $f
```

After the hook fires, inspect the newest file under `/tmp/` (or `C:\Temp\`):

```sh
cat /tmp/tm-hook-*.json | python3 -m json.tool   # pretty-print
```

---

## Codex CLI

**Adapter:** `internal/harness/codex.go`
**Installed config:** `.codex-plugin/hooks/hooks.json` (written by `tm init --harness codex`)

### Echo-hook JSON

Replace `.codex-plugin/hooks/hooks.json` with the following to capture raw payloads.
Restore the original content when verification is complete.

```json
{
  "PreToolUse": [{
    "matcher": "^(Bash|apply_patch)$",
    "hooks": [{ "type": "command", "command": "sh -c 'cat > /tmp/tm-hook-pre-$(date +%s).json'" }]
  }],
  "PostToolUse": [{
    "matcher": "^(Bash|apply_patch)$",
    "hooks": [{ "type": "command", "command": "sh -c 'cat > /tmp/tm-hook-post-$(date +%s).json'" }]
  }]
}
```

### Actions to trigger

**Check A — apply_patch coverage:** Ask Codex to make a small file edit (e.g.,
"add a comment to README.md"). If Codex uses the `apply_patch` tool internally,
a PreToolUse and PostToolUse file will appear. If only Bash is used, no file
will appear for the edit.

**Check B — exit code path:** Ask Codex to run a shell command that exits
non-zero, e.g., "run `exit 42`". Confirm a PostToolUse file appears and inspect
its `tool_response` object.

### What to confirm

**(a) apply_patch hook coverage**

```sh
ls /tmp/tm-hook-pre-*.json /tmp/tm-hook-post-*.json
grep -l '"tool_name":"apply_patch"' /tmp/tm-hook-*.json
```

Expected: at least one file exists with `"tool_name":"apply_patch"`. If no such
file appears, Codex does not emit PreToolUse/PostToolUse for `apply_patch` — see
remediation A below.

**(b) Exit code location**

```sh
grep -o '"exit_code":[0-9-]*' /tmp/tm-hook-post-*.json
```

Expected: output like `"exit_code":42`. The adapter reads
`tool_response.exit_code` (see `codex.go` line 23). If the grep is empty but
the file contains an exit code elsewhere, find it with:

```sh
python3 -c "import sys,json; d=json.load(open(sys.argv[1])); print(json.dumps(d,indent=2))" /tmp/tm-hook-post-*.json | grep -i exit
```

Then adjust only the `Parse` function in `internal/harness/codex.go` to match
the actual path — see remediation B below.

### Remediation

**A — apply_patch not covered:** Change the matcher in
`.codex-plugin/hooks/hooks.json` (and the template in
`internal/cli/install_codex.go`) from `^(Bash|apply_patch)$` to `^Bash$`. Note
in the installer comment that file-edit retrieval via `apply_patch` is
unavailable until Codex upstream emits those hook events.

**B — exit code at a different path:** Update only the `ToolResponse` struct and
the `ev.Failed` assignment in `codex.go`'s `Parse` function. No other file
depends on this wire shape.

---

## Copilot CLI

**Adapter:** `internal/harness/copilot.go`
**Installed config:** `.github/hooks/teammemory.json` (written by `tm init --harness copilot`)

### Echo-hook JSON

Replace `.github/hooks/teammemory.json` with the following to capture raw
payloads. Restore when done.

```json
{
  "version": 1,
  "hooks": {
    "postToolUse": [{
      "type": "command",
      "bash": "sh -c 'cat > /tmp/tm-hook-post-$(date +%s).json'"
    }],
    "postToolUseFailure": [{
      "type": "command",
      "bash": "sh -c 'cat > /tmp/tm-hook-fail-$(date +%s).json'"
    }]
  }
}
```

### Actions to trigger

**Check A — failure signal:** Ask Copilot to run a shell command that exits
non-zero, e.g., "run `exit 99`". Observe whether a `tm-hook-fail-*.json` file
appears (meaning Copilot fired a separate `postToolUseFailure` event) or only a
`tm-hook-post-*.json` appears (meaning failure is signalled inline via
`toolResult.exitCode`).

**Check B — additionalContext on output:** Create a temporary hook script that
emits a probe value on stdout, to confirm the agent actually sees it:

```sh
cat > /tmp/probe-hook.sh << 'EOF'
#!/bin/sh
printf '{"additionalContext":"TM-PROBE-12345"}\n'
EOF
chmod +x /tmp/probe-hook.sh
```

Register it as the `postToolUse` hook in the echo JSON above by replacing the
`bash` value with `"/tmp/probe-hook.sh"`. Then ask Copilot to run any command
that succeeds, and inspect the subsequent agent turn for the probe string
`TM-PROBE-12345` in the transcript or context window.

### What to confirm

**(a) Failure signal source**

```sh
# Did a dedicated postToolUseFailure file appear?
ls /tmp/tm-hook-fail-*.json 2>/dev/null && echo "FAILURE EVENT" || echo "no failure event"

# Or is exit code present inline in the postToolUse payload?
grep -o '"exitCode":[0-9-]*' /tmp/tm-hook-post-*.json
```

The adapter (`copilot.go` lines 37–40) handles both paths: it checks
`hook_event_name == "postToolUseFailure"` first, then falls back to
`toolResult.exitCode`. Confirm that at least one of these signals is present in
the captured payload.

**(b) additionalContext honored by script hook**

After running the probe-hook check above:

```sh
# Search the Copilot agent transcript (location varies by CLI version):
grep -r "TM-PROBE-12345" ~/.copilot/ 2>/dev/null || echo "not found in ~/.copilot"
```

If the probe string does not appear anywhere the agent could read it, the script
(non-SDK) hook path silently discards `additionalContext` output. In that case
use remediation C below.

### Remediation

**C — additionalContext dropped by script path:** The packaging in
`internal/cli/install_copilot.go` must switch from `"type": "command"` script
hooks to the Copilot SDK hook variant (which surfaces `additionalContext` through
the SDK's return value). Adjust only the hook JSON template written by
`installCopilot` and the corresponding adapter `Render` output shape in
`internal/harness/copilot.go`. No other file depends on this output field.

---

## Cursor CLI

**Adapter:** `internal/harness/cursor.go`
**Installed config:** `.cursor/hooks.json` (written by `tm init --harness cursor`)

### Echo-hook JSON

Replace `.cursor/hooks.json` with the following to capture raw payloads.
Restore the original content when verification is complete.

```json
{
  "version": 1,
  "hooks": {
    "afterShellExecution": [{ "command": "sh -c 'cat > /tmp/tm-hook-shell-$(date +%s).json'" }],
    "postToolUseFailure":  [{ "command": "sh -c 'cat > /tmp/tm-hook-fail-$(date +%s).json'" }],
    "afterFileEdit":       [{ "command": "sh -c 'cat > /tmp/tm-hook-edit-$(date +%s).json'" }]
  }
}
```

### Actions to trigger

**Check A — afterShellExecution payload:** Ask Cursor to run a shell command
that exits zero, e.g., "run `echo hello`". Confirm a `tm-hook-shell-*.json`
file appears. Inspect it for the `command` and `hook_event_name` fields.

**Check B — failure event:** Ask Cursor to run a command that exits non-zero,
e.g., "run `exit 7`". Confirm a `tm-hook-fail-*.json` file appears (the
dedicated `postToolUseFailure` event) and inspect its fields.

**Check C — file-edit event:** Ask Cursor to make a small file edit. Confirm
whether `afterFileEdit` fires (a `tm-hook-edit-*.json` file appears) and what
fields it carries. Note that Cursor does not document a `beforeFileEdit` event,
so if no pre-edit hook fires for file edits, requirement-blocking on file edits
is shell-only on Cursor (there is no pre-edit block path for `afterFileEdit`).

**Check D — additional_context model-visibility:** Create a probe hook script
that emits a probe value on stdout, to confirm the agent actually sees it:

```sh
cat > /tmp/probe-hook.sh << 'EOF'
#!/bin/sh
printf '{"additional_context":"TM-PROBE-CURSOR-12345"}\n'
EOF
chmod +x /tmp/probe-hook.sh
```

Register it as the `afterShellExecution` hook by replacing the `command` value
with `"/tmp/probe-hook.sh"`. Run any passing shell command and inspect the
subsequent agent turn for `TM-PROBE-CURSOR-12345`.

### What to confirm

**(a) Field names in afterShellExecution**

```sh
grep -o '"hook_event_name":"[^"]*"' /tmp/tm-hook-shell-*.json
grep -o '"command":"[^"]*"' /tmp/tm-hook-shell-*.json
```

Expected: `"hook_event_name":"afterShellExecution"` and a non-empty `"command"`
value. The adapter (`cursor.go`) reads these exact snake_case field names.

**(b) Field names in postToolUseFailure**

```sh
grep -o '"hook_event_name":"[^"]*"' /tmp/tm-hook-fail-*.json
grep -o '"command":"[^"]*"' /tmp/tm-hook-fail-*.json
```

Expected: `"hook_event_name":"postToolUseFailure"`. The adapter uses this to
set `Failed=true` for the command outcome.

**(c) File-edit pre-event coverage**

```sh
# Did a beforeFileEdit file appear?
ls /tmp/tm-hook-edit-*.json 2>/dev/null && echo "AFTER-EDIT EVENT" || echo "no file-edit event"
```

If no `beforeFileEdit` event fires for file edits (expected — Cursor only
documents `afterFileEdit`), note that edit-time requirement enforcement is
shell-only on Cursor. Adjust only the installer template comment in
`internal/cli/install_cursor.go` and the adapter note in `cursor.go` to
reflect this; no other file depends on this behavior.

**(d) additional_context model-visibility**

After running the probe-hook check:

```sh
# Search the Cursor agent transcript (location varies by CLI version):
grep -r "TM-PROBE-CURSOR-12345" ~/.cursor/ 2>/dev/null || echo "not found in ~/.cursor"
```

### Remediation

**If `beforeShellExecution`/`preToolUse` does not fire for file edits:** This
is expected on current Cursor. Note in `install_cursor.go` that edit-time
requirement enforcement on Cursor fires only for shell commands (via
`beforeShellExecution`), not for file edits. Adjust only the installer template
comment and the adapter note in `cursor.go`; no engine changes are needed.

**If `additional_context` is not model-visible:** Switch the Cursor adapter's
`Render` output to the field name that the live harness exposes for model-visible
injection. Adjust only `cursor.go`'s `Render` function and the installer note.

---

## Gemini CLI

**Adapter:** `internal/harness/gemini.go`
**Installed config:** `.gemini/settings.json` (written by `tm init --harness gemini`)

### Echo-hook JSON

Replace the `hooks` block in `.gemini/settings.json` with the following to
capture raw `AfterTool` payloads. Restore when done. Keep `mcpServers` in place.

```json
{
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": {
    "AfterTool": [{ "command": "sh -c 'cat > /tmp/tm-hook-aftertool-$(date +%s).json'" }]
  }
}
```

### Actions to trigger

**Check A — passing command payload:** Ask Gemini to run a shell command that
exits zero, e.g., "run `echo hello`". Confirm a `tm-hook-aftertool-*.json`
file appears. Inspect its full structure.

**Check B — failing command payload:** Ask Gemini to run a shell command that
exits non-zero, e.g., "run `exit 9`". Confirm a second file appears with a
non-empty `tool_response.error` field.

**Check C — additionalContext model-visibility:** Create a probe hook script:

```sh
cat > /tmp/probe-hook-gemini.sh << 'EOF'
#!/bin/sh
printf '{"hookSpecificOutput":{"additionalContext":"TM-PROBE-GEMINI-12345"}}\n'
EOF
chmod +x /tmp/probe-hook-gemini.sh
```

Register it as the `AfterTool` hook, run a passing command, and inspect the
subsequent agent turn for `TM-PROBE-GEMINI-12345`. Note that `systemMessage`
is user-only and must NOT be used; only `hookSpecificOutput.additionalContext`
reaches the model.

### What to confirm

**(a) tool_response.error on failure**

```sh
grep -o '"error":"[^"]*"' /tmp/tm-hook-aftertool-*.json
```

Expected: non-empty `"error"` value in the failing-command file and empty (or
absent) in the passing-command file. The adapter (`gemini.go`) sets
`Failed = raw.ToolResponse.Error != ""`.

**(b) Schema matches the pinned Gemini release tag**

The Gemini CLI schema differs between the `main` branch and released tags.
Verify the captured payload's top-level structure matches what `gemini.go`
expects (`session_id`, `tool_name`, `tool_input.command`, `tool_input.file_path`,
`tool_response.error`). If the pinned release tag uses different field names,
adjust only the `Parse` function structs in `internal/harness/gemini.go`.

**(c) additionalContext model-visibility**

After running the probe-hook check:

```sh
# Gemini CLI transcript location varies; search broadly:
grep -r "TM-PROBE-GEMINI-12345" ~/.gemini/ 2>/dev/null || echo "not found in ~/.gemini"
```

If the probe string does not appear where the model can read it but `systemMessage`
does appear in a model-visible position, report this finding — the adapter
currently uses `hookSpecificOutput.additionalContext` per the published Gemini
hook spec; if that field is not model-visible on the pinned release, switch
`gemini.go`'s `Render` to emit the correct field name. Adjust only
`internal/harness/gemini.go`.

### Remediation

**If `tool_response.error` is at a different path:** Update only the
`ToolResponse` struct in `gemini.go`'s `Parse` function to match the actual
field path. No other file depends on this wire shape.

**If `hookSpecificOutput.additionalContext` is NOT model-visible on the pinned
tag:** Switch the adapter's `Render` output shape to whichever field the
pinned release exposes for model-visible context injection. Adjust only
`internal/harness/gemini.go`'s `Render` function and the installer note in
`internal/cli/install_gemini.go`. No engine changes are needed.

---

## Checklist

A verifier who completes the recipes above should tick the items they confirmed
and report the results so the `VERIFY` annotations in the adapter source and
`prd.md §10.6` can be resolved.

- [ ] **Codex apply_patch coverage** — PreToolUse/PostToolUse hook fires for
  `apply_patch` file edits (not Bash-only). Confirmed by `grep '"tool_name":"apply_patch"'`.
- [ ] **Codex exit-code path** — exit code is present at `tool_response.exit_code`
  in the PostToolUse payload. Confirmed by `grep -o '"exit_code":[0-9-]*'`.
- [ ] **Copilot fail-signal source** — failure arrives via
  `hook_event_name == "postToolUseFailure"` and/or `toolResult.exitCode` (camelCase).
  Confirmed by inspecting captured payload files.
- [ ] **Copilot additionalContext honored** — a `postToolUse` script hook's
  `{"additionalContext":"..."}` stdout reaches the agent's context window.
  Confirmed by finding the probe string `TM-PROBE-12345` in the agent transcript.
- [ ] **Cursor field names** — `afterShellExecution` and `postToolUseFailure`
  payloads carry `hook_event_name` (snake_case) and `command` (snake_case) as
  the adapter expects. Confirmed by grepping captured payload files.
- [ ] **Cursor edit-blocking coverage** — confirmed whether Cursor fires a
  pre-edit event for file edits. If `beforeFileEdit` does not fire (expected),
  noted that edit-time requirement enforcement is shell-only on Cursor; installer
  template and adapter comment updated accordingly.
- [ ] **Gemini pinned-tag schema** — captured `AfterTool` payload structure
  matches `gemini.go`'s expected fields (`tool_response.error`, `tool_input.command`,
  etc.) on the pinned Gemini CLI release tag (not `main`).
- [ ] **Gemini additionalContext model-visibility** — `hookSpecificOutput.additionalContext`
  in the adapter's `Render` output reaches the model's context window (not
  user-only). Confirmed by finding probe string `TM-PROBE-GEMINI-12345` in the
  agent transcript.
