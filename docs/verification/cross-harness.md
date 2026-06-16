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
**Installed config:** `<repo>/.codex/hooks.json` (written by `tm init --harness codex`)

**Status — confirmed against OpenAI's published hook docs** (https://developers.openai.com/codex/hooks):
Codex loads `<repo>/.codex/hooks.json` (and `~/.codex/hooks.json`) with the event
map wrapped under a top-level `hooks` key; file edits report `tool_name:
"apply_patch"` (a matcher may also name `Edit`/`Write`); the exit code sits at
`tool_response.exit_code`. The recipe below is the belt-and-braces live re-check
that the gated harness smoke test automates. Repo hooks require trust on first
run — use `--dangerously-bypass-hook-trust` for non-interactive capture.

**Codex hook firing — verified working (CLI 0.139.0, 2026-06-15):** Codex DOES
fire hooks, including headless `codex exec`, but **only after the repo's hooks are
trusted once interactively**. On first interactive `codex` run in the repo, a
"Hooks need review — N hooks are new or changed" prompt appears; choosing **"Trust
all and continue"** persists a per-hook `sha256` under `[hooks.state]` in
`~/.codex/config.toml`, keyed by `<hooks.json abs path>:<event>:<group>:<idx>`.
With that trust recorded, `codex exec` fires `SessionStart`/`UserPromptSubmit`/
`PreToolUse`/`PostToolUse`/`Stop` headlessly — and notably does **not** need
`--dangerously-bypass-hook-trust` once trusted. (`--dangerously-bypass-hook-trust`
on a fresh, untrusted repo does **not** substitute for that persisted trust in
this version, which is why the per-run-temp-repo `TestLive/codex` can't fire and
is skipped/gated on a pre-trusted repo. See the manual recipe at the end of this
section.)

**Requirement blocking — VERIFIED live (2026-06-16):** `TestLiveCodexRequirementBlock`
(gated on a trusted `TM_CODEX_BLOCK_REPO`) drives real `codex exec` against an
active path-scoped requirement; the protected file stays unwritten AND the
requirement is surfaced, so codex honors a `PreToolUse` deny — even under
`--dangerously-bypass-approvals-and-sandbox`. This took two fixes the test
uncovered: (a) codex's file-edit tool is **`apply_patch`**, and the edited path is
inside the patch text at `tool_input.command` (`*** Add/Update/Delete File:
<path>`), not a `file_path` field — `codex.go` now parses it (previously the patch
blob was mis-read as a *command*, so no path matched and codex file-edit blocking
silently no-opped); (b) `check-action` now resolves a repo-relative hook path
against the repo root (codex emits relative `apply_patch` paths) via the shared
`relPath` helper. Unit-pinned by `TestCodexParseApplyPatchExtractsPath`.

**Codex live wire-shape findings (correcting the docs-based assumptions):**
- Shell tool reports `tool_name: "Bash"` with the command at `tool_input.command`
  (so the `^(Bash|apply_patch)$` matcher is correct).
- A **successful** `PostToolUse` carries `tool_response` as a **plain string** (the
  command output, e.g. `"hello\r\n"`), NOT an object with `exit_code`. **Fixed:**
  `codex.go` now decodes `tool_response` as `json.RawMessage` and tolerates both a
  string (passing) and an object (`exit_code` checked, forward-compat).
- A **failing** tool call (both a thrown error and a clean non-zero `exit 3`) emits
  **no `PostToolUse` at all** — only `PreToolUse` then `Stop`. **Resolved:** the
  `codex` row's `PostToolFailureSensor` was set to **no**, and the `fail_pass_nudge`
  scenario is not applicable to codex. (Claude Code was later found to share this
  exact behavior — see the Claude section below — and was flipped to `no` too.)

**Automated codex live test (`TestLive/codex`).** Because each `TestLive` run
uses a fresh, untrusted temp repo, codex's subtest can't fire there. Instead it is
gated on `TM_CODEX_LIVE_REPO` — a repo prepared once and trusted interactively:

```sh
# 1. Scaffold: builds the recorder into <repo> and writes .codex/hooks.json
#    (every event → recorder; catch-all matcher on the tool events).
TM_CODEX_LIVE_REPO=/path/to/codex-live \
  go test -tags harness_live ./e2e/harness/ -run TestSetupCodexLiveRepo -v

# 2. Trust it ONCE (the only manual step): run codex interactively in that repo
#    and choose "Hooks need review" → "Trust all and continue".
cd /path/to/codex-live && codex   # then /quit after the trust prompt

# 3. Run the live test — now codex exec fires headlessly against the trusted repo.
TM_CODEX_LIVE_REPO=/path/to/codex-live \
  go test -tags harness_live ./e2e/harness/ -run TestLive/codex -v
```

With `TM_CODEX_LIVE_REPO` unset, `TestLive/codex` **skips** (with these
instructions) rather than failing. The marker file is written outside the repo,
so the trusted `hooks.json` is never modified (which would invalidate the trust
hash).

### Echo-hook JSON

Replace `<repo>/.codex/hooks.json` with the following to capture raw payloads
(note the top-level `hooks` wrapper). Restore the original content when done.

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "^(Bash|apply_patch)$",
      "hooks": [{ "type": "command", "command": "sh -c 'cat > /tmp/tm-hook-pre-$(date +%s).json'" }]
    }],
    "PostToolUse": [{
      "matcher": "^(Bash|apply_patch)$",
      "hooks": [{ "type": "command", "command": "sh -c 'cat > /tmp/tm-hook-post-$(date +%s).json'" }]
    }]
  }
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

**A — apply_patch not covered:** Change the matcher in `<repo>/.codex/hooks.json`
(and the template in `internal/cli/install_codex.go`) from `^(Bash|apply_patch)$`
to `^Bash$`. Note in the installer comment that file-edit retrieval via
`apply_patch` is unavailable until Codex upstream emits those hook events. (Docs
indicate `apply_patch` is covered, so this remediation is unlikely to be needed.)

**B — exit code at a different path:** Update only the `ToolResponse` struct and
the `ev.Failed` assignment in `codex.go`'s `Parse` function. No other file
depends on this wire shape.

---

## Copilot CLI

**Adapter:** `internal/harness/copilot.go`
**Installed config:** `.github/hooks/teammemory.json` (written by `tm init --harness copilot`)

**Status — confirmed against GitHub's docs** (https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks):
hooks load from `.github/hooks/*.json`; each hook entry must carry **both** a
`bash` key (Linux/macOS) and a `powershell` key (Windows); the supported events
are `sessionStart`/`sessionEnd`/`userPromptSubmitted`/`preToolUse`/`postToolUse`/`errorOccurred`
(plus `agentStop`) — there is **no** `postToolUseFailure` event.

**Live failure shape — RESOLVED (copilot 1.0.62, 2026-06-15):** a failed shell
command does **not** surface via `errorOccurred` or a structured `toolResult.exitCode`.
Copilot fires `postToolUse` with `toolResult.resultType: "success"` (the TOOL ran)
even when the command exited non-zero; the real exit status is the trailing
`exit code N` inside `toolResult.textResultForLlm`, e.g.
`"...<shellId: 0 completed with exit code 1>"`. The adapter now parses that exit
code (keeping the `errorOccurred`/`error`/`toolResult.exitCode` branches for
forward-compat). Pinned by `harness.TestCopilotExitCodeFromResultText` (unit) and
`TestLiveCommandFailureSensed/copilot` (live).

**additionalContext model-visibility — VERIFIED (2026-06-16):** a `postToolUse`
hook's `additionalContext` reaches the model. Informational advisory content (tm's
real shape) is surfaced and trusted; an imperative instruction is flagged as a
prompt-injection attempt and declined — so keep injected advisory content
descriptive.

**Requirement blocking — VERIFIED live (2026-06-16):** `TestLiveRequirementBlock`
seeds a path-scoped requirement and drives real `copilot --allow-all-tools`; the
protected file stays unwritten AND the requirement is surfaced in the journal, so
Copilot honors a `preToolUse` deny — the `--allow-all-tools` bypass flag does
**not** swallow it. Nothing left live-pending for Copilot.

### Echo-hook JSON

Replace `.github/hooks/teammemory.json` with the following to capture raw
payloads (Windows uses the `powershell` key; substitute a capture script that
writes stdin to a file). Restore when done.

```json
{
  "version": 1,
  "hooks": {
    "postToolUse": [{
      "type": "command",
      "bash": "sh -c 'cat > /tmp/tm-hook-post-$(date +%s).json'",
      "powershell": "powershell -NoProfile -File C:\\tmp\\capture.ps1 postToolUse"
    }],
    "errorOccurred": [{
      "type": "command",
      "bash": "sh -c 'cat > /tmp/tm-hook-fail-$(date +%s).json'",
      "powershell": "powershell -NoProfile -File C:\\tmp\\capture.ps1 errorOccurred"
    }]
  }
}
```

### Actions to trigger

**Check A — failure signal:** Ask Copilot to run a shell command that exits
non-zero, e.g., "run `exit 99`". Observe whether an `errorOccurred` payload
appears (a separate failure event) and/or the `postToolUse` payload carries an
`error` field or an exit code.

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
# Did a dedicated errorOccurred file appear?
ls /tmp/tm-hook-fail-*.json 2>/dev/null && echo "ERROR EVENT" || echo "no error event"

# Or is failure inline in the postToolUse payload (error field / exit code)?
grep -oE '"(error|exitCode)":[^,}]*' /tmp/tm-hook-post-*.json
```

The adapter's `Parse` (`internal/harness/copilot.go`) treats a tool as failed
when the event is `errorOccurred`, the `error` field is non-empty, a structured
`toolResult.exitCode` is non-zero, or — the live shell shape — a non-zero
`exit code N` is parsed from `toolResult.textResultForLlm`. The live payload
carries the last of these; the others are kept for forward-compat.

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

**Status — live firing confirmed (cursor 2026.06.12):** The headless
`cursor-agent` CLI (installed as `agent`) fires `.cursor/hooks.json` hooks when
driven with `agent -p --force --trust "<prompt>"`. Two headless limitations apply:
(1) the headless CLI does **not** fire `stop` or `beforeSubmitPrompt`, so
Cursor's Stop-nudge and prompt fixtures stay authored rather than live-captured;
(2) `afterShellExecution` carries no exit code, so `cmd-pass` also stays
authored. See `e2e/harness/CURSOR_NOTES.md` for full live-tier notes.

**Windows BOM (found via `TestLiveRealTmRecording`, 2026-06-15):** on Windows,
`cursor-agent` prepends a UTF-8 BOM (`EF BB BF`) to hook stdin. Go's JSON decoder
rejects a leading BOM (`invalid character 'ï' looking for beginning of value`),
which silently broke every cursor hook (the recorder masked it; real `tm`
surfaced it). Fixed in `internal/harness` — the shared `decodeJSON` helper strips
a leading BOM before decoding, and all five adapters route through it. Regression
test: `TestCursorParseToleratesBOM`.

**Requirement blocking — VERIFIED live (2026-06-16):** `TestLiveRequirementBlock/cursor`
seeds a **command-scoped** requirement (Cursor's pre-tool block is shell-only — there
is no pre-edit hook) and drives real `agent --force` to run a shell command with an
observable side effect; the marker file is never created AND the requirement is
surfaced, so Cursor honors a `beforeShellExecution` deny — `--force` does **not**
swallow it.

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
# Did an afterFileEdit file appear?
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

**If `beforeFileEdit` does not fire for file edits:** This
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

**Status — live firing confirmed (gemini CLI, verified 2026-06-15):** Driven with
`gemini -p "<prompt>" --yolo`, the installed `.gemini/settings.json` hooks fire
for all four events. The captured payloads confirm: `hook_event_name` is
`BeforeTool`/`AfterTool`/`BeforeAgent`/`AfterAgent`; the shell tool reports
`tool_name: "run_shell_command"` with the command at `tool_input.command`; the
`AfterTool` success payload carries `tool_response.llmContent`/`returnDisplay`.
**Schema gotcha:** each event must use the nested group shape — `[{ "matcher":
<regex>, "hooks": [{ "type": "command", "command": … }] }]`; a flat `[{ "command":
… }]` entry is rejected at load ("Discarding invalid hook definition") and never
fires. Tool events (`BeforeTool`/`AfterTool`) require a matcher;
`BeforeAgent`/`AfterAgent` omit it.

**Live failure shape — RESOLVED (2026-06-15):** a failed shell command does **not**
populate `tool_response.error` (that field is absent). Gemini *does* fire
`AfterTool` on failure, and the exit status appears as an `Exit Code: N` line
inside `tool_response.llmContent` (e.g. `"Output: ...\nExit Code: 1\nProcess Group
PGID: 3172"`) — present only on a non-zero exit; a successful command's
`llmContent` has no such line. The adapter now parses that line (keeping the
`tool_response.error` branch for forward-compat). Pinned by
`harness.TestGeminiExitCodeFromLlmContent` (unit) and
`TestLiveCommandFailureSensed/gemini` (live).

**additionalContext model-visibility — VERIFIED (2026-06-16):** the model echoed
the injected `hookSpecificOutput.additionalContext` reference code in its visible
reply (no injection-suspicion).

**Requirement blocking — VERIFIED live (2026-06-16):** `TestLiveRequirementBlock`
seeds a path-scoped requirement and drives real `gemini --yolo`; the protected
file stays unwritten AND the requirement is surfaced in the journal, so Gemini
honors a `BeforeTool` deny — `--yolo` auto-approval does **not** override it.
Nothing left live-pending for Gemini.

### Echo-hook JSON

Replace the `hooks` block in `.gemini/settings.json` with the following to
capture raw payloads (note the nested `matcher`+`hooks` group shape — a flat
entry will not fire). Restore when done. Keep `mcpServers` in place.

```json
{
  "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } },
  "hooks": {
    "AfterTool": [{ "matcher": ".*", "hooks": [{ "type": "command", "command": "sh -c 'cat > /tmp/tm-hook-aftertool-$(date +%s).json'" }] }]
  }
}
```

### Actions to trigger

**Check A — passing command payload:** Ask Gemini to run a shell command that
exits zero, e.g., "run `echo hello`". Confirm a `tm-hook-aftertool-*.json`
file appears. Inspect its full structure.

**Check B — failing command payload:** Ask Gemini to run a shell command that
exits non-zero, e.g., "run `exit 9`". Confirm a second file appears. Note the
failure is **not** in `tool_response.error` (absent) — it is the `Exit Code: N`
line inside `tool_response.llmContent`.

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

**(a) failure signal in llmContent (not tool_response.error)**

```sh
grep -o 'Exit Code: [0-9]*' /tmp/tm-hook-aftertool-*.json
```

Expected: an `Exit Code: N` line (N non-zero) inside `tool_response.llmContent`
in the failing-command file, and no such line in the passing-command file. The
adapter (`gemini.go`) parses that line; `tool_response.error` is absent on the
live payload and is kept only as a forward-compat branch.

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

## Claude Code

**Adapter:** `internal/harness/claude.go`
**Installed config:** `.claude/settings.json` (written by `tm init`)

Claude Code is the reference harness, but one wire-shape assumption proved wrong
and is worth recording here.

**Failure sensing — RESOLVED (CLI 2.1.177, 2026-06-15):** Claude Code fires
`PostToolUse` only after a tool completes **successfully**. A failing Bash command
emits `PreToolUse` then **no `PostToolUse` at all** (confirmed across repeated live
runs: only the *successful* retry and the file `Write` produced `PostToolUse`).
A successful Bash `tool_response` is `{stdout,stderr,interrupted,isImage,noOutputExpected}`
— with **no `exit_code`**. So command-failure sensing cannot fire on Claude, exactly
like Codex. The capability matrix sets claude `PostToolFailureSensor = no`, the
`fail_pass_nudge` scenario is not applicable, and the adapter's `exit_code` check is
retained only for forward-compat. Pinned by `harness.TestClaudeSuccessPostToolHasNoExitCode`.
**Re-check by ~2026-08-15** alongside Codex: if a later version emits a `PostToolUse`
on failure, re-enable the capability and restore the scenario.

---

## Checklist

A verifier who completes the recipes above should tick the items they confirmed
and report the results so the `VERIFY` annotations in the adapter source and
`prd.md §10.6` can be resolved.

- [x] **Codex apply_patch coverage** — file edits go through `apply_patch` (a
  matcher may also name `Edit`/`Write`; hook input reports `tool_name:
  "apply_patch"`). Confirmed via OpenAI hook docs; gated live smoke re-checks.
- [x] **Codex exit-code path** — exit code is at `tool_response.exit_code` in the
  PostToolUse payload. Confirmed via OpenAI hook docs; gated live smoke re-checks.
- [x] **Copilot fail-signal source** — RESOLVED live (1.0.62): failure is the
  trailing `exit code N` in `toolResult.textResultForLlm` (`resultType` is
  `"success"` even on failure; no structured `exitCode`). Adapter parses it; pinned
  by `TestCopilotExitCodeFromResultText` + `TestLiveCommandFailureSensed/copilot`.
- [x] **Copilot additionalContext honored** — VERIFIED live (2026-06-16): a
  `postToolUse` script hook's `{"additionalContext":"..."}` reaches the model. With
  **informational** content (tm's real shape — naming a relevant memory) the model
  surfaces and trusts it; with an **imperative** instruction it flags the hook
  side-channel as a prompt-injection attempt and declines. Keep advisory content
  descriptive, not commanding. Verified by one-time probe (live model behavior is
  non-deterministic — not a CI gate).
- [ ] **Cursor field names** — `afterShellExecution` and `postToolUseFailure`
  payloads carry `hook_event_name` (snake_case) and `command` (snake_case) as
  the adapter expects. Confirmed by grepping captured payload files.
- [ ] **Cursor edit-blocking coverage** — confirmed whether Cursor fires a
  pre-edit event for file edits. If `beforeFileEdit` does not fire (expected),
  noted that edit-time requirement enforcement is shell-only on Cursor; installer
  template and adapter comment updated accordingly.
- [x] **Gemini hook schema + firing** — live `gemini -p --yolo` payloads confirm
  the nested group config shape is required, hooks fire for all four events,
  `tool_name: "run_shell_command"`, and the command sits at `tool_input.command`
  as `gemini.go` expects.
- [x] **Gemini fail-signal source** — RESOLVED live: failure is the `Exit Code: N`
  line in `tool_response.llmContent` (`tool_response.error` is absent). Adapter
  parses it; pinned by `TestGeminiExitCodeFromLlmContent` +
  `TestLiveCommandFailureSensed/gemini`.
- [x] **Claude/Codex failure sensing** — RESOLVED live: both fire `PostToolUse` on
  success only, so `PostToolFailureSensor = no` for both. Re-check by ~2026-08-15.
- [x] **Requirement blocking honored under bypass flags** — VERIFIED live
  (2026-06-16) for **all five harnesses**: a scoped requirement blocks the action
  and the bypass run flag (`--dangerously-skip-permissions`/`--allow-all-tools`/
  `--yolo`/`--force`/`--dangerously-bypass-approvals-and-sandbox`) does not swallow
  the hook deny. Cursor uses a command-scoped requirement (pre-tool block is
  shell-only). Codex (`TestLiveCodexRequirementBlock`, gated on one-time trust)
  needed the `apply_patch` path-extraction + `check-action` relative-path fixes
  above. `TestLiveRequirementBlock` covers Claude/Copilot/Gemini/Cursor.
- [x] **Gemini additionalContext model-visibility** — VERIFIED live (2026-06-16):
  `hookSpecificOutput.additionalContext` reaches the model (it echoed the injected
  reference code in its visible reply, no injection-suspicion). Verified by
  one-time probe (live model behavior is non-deterministic — not a CI gate).
