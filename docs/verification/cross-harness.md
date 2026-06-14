# Cross-harness live-payload verification recipes

## Purpose

TeamMemory's Codex and Copilot CLI adapters were built from published specs and
source inspection, but two assumptions cannot be validated by unit tests: they
depend on wire shapes that only a live harness emits. The checks below require
the respective CLI to be installed and configured in a real project. Once a
verifier runs each recipe and confirms the payload matches what the adapter
expects, they can tick the corresponding `VERIFY (spec §10)` annotation in
`internal/harness/codex.go` and `internal/harness/copilot.go`, and update the
status note in `prd.md §10`.

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

## Checklist

A verifier who completes the recipes above should tick the items they confirmed
and report the results so the `VERIFY` annotations in the adapter source and
`prd.md §10` can be resolved.

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
