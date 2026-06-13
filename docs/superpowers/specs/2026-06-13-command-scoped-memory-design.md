# Design: Command-scoped memory & Bash-time delivery

- **Date:** 2026-06-13
- **Status:** Approved (brainstorming) — pending spec review, then implementation plan
- **PRD sections touched:** §5.1, §8.1, §8.5, §9.1, §10.1, §10.3, §11
- **Note:** This file is an ephemeral working note (AGENTS.md). The durable spec
  change lands in `prd.md` in the same commit as the code; this file is removed
  before the work is pushed.

## 1. Problem

TeamMemory's headline claim is *deterministic delivery over voluntary recall*
(§6.1, decision §18.9): the Claude Code `PreToolUse` hook guarantees
`check_action` happens without the agent choosing to call it. But the hook only
matches `Edit|Write|MultiEdit` (`internal/cli/plugin.go:22`, §10.1) and is keyed
on `tool_input.file_path` (`internal/cli/checkaction.go:137-142`).

A whole class of memory is inherently **command-time**, not edit-time:

> "Running `pytest` fails unless `DATABASE_URL` is set / `make seed` ran first."
> "`assistant jira create` fails unless `--project` is passed."

These are squarely memory-worthy (`constraint` / `failed_attempt`, §5.1), but an
agent invoking them through the **Bash** tool gets no deterministic delivery: the
hook never fires, and a Bash command has no edit path for the path-glob channel
to match. The memory surfaces only if the agent *voluntarily* calls
`tm_check_action` — exactly the unreliable path the hook exists to replace (§15).

So determinism reaches edits but not commands. This design closes that gap.

## 2. Why `scope.commands` (not FTS-only)

Retrieval has two independent candidate channels (`internal/retrieve/retrieve.go:124-128`):

- **Scope-glob (structural):** memory `scope.paths` globs tested against the
  action's concrete paths — containment, with a specificity score.
- **FTS (lexical):** a free-text `Description` token-matched against
  title/summary/guidance — fuzzy token overlap, no structural guarantee.

The hook feeds only the scope channel today (`Query{Paths: [...]}`).

A command could be routed through either:

- **FTS-only:** pass the command string as `Query.Description`. Cheap (the engine
  already does FTS), but the provisional gate
  (`retrieve.go:145-147`) surfaces provisional memories **only on a structural
  scope match, never on FTS-only** — a deliberate trust rule (fuzzy matches may
  only surface already-validated memories, to avoid noise). So under FTS-only a
  *provisional* command lesson never surfaces at command time and can't be
  confirmed there: command matching becomes a delivery boost for already-active
  memories, not a validation path.
- **`scope.commands` (chosen):** a structural command-match channel. Command
  matches behave like scope matches, so provisional command lessons surface as
  caution at command time and complete the propose → confirm → activate flywheel
  (§6.4) on their own — even with no path anchor.

Everything else (independence, derived state, the <100ms budget, the SQLite
index) is identical between the options. The flywheel is the deciding factor.

## 3. Design

### 3.1 Data model (§9.1)

Add an optional `commands` list to `scope`, a sibling of `paths`:

```yaml
scope:
  paths: []                    # may be empty for a pure command lesson
  commands:
    - "assistant jira create *"
```

A memory may carry `paths`, `commands`, or both. `anchors` stay optional — a
pure command lesson has no anchored file, so anchor-drift annotation (§8.6)
simply does not apply to it.

### 3.2 Matching — token-aware structural channel (§11)

A third candidate channel, parallel to scope-glob and FTS:

- **Tokenize** the command: split on whitespace; strip leading `VAR=val`
  environment-assignment prefixes.
- Match pattern tokens **positionally**; a trailing `*` matches one-or-more
  remaining tokens.
- **Leading-subcommand only** — flags and their order are not matched:
  `assistant jira create *` matches both `… --project X` and `… X --project`.

Because the match is structural (containment, not token overlap), provisional
command lessons surface as caution at command time — the property FTS-only
lacked.

**Deliberate v1 limits (documented, like the anchor caveats):**

- Match the leading subcommand path only; do **not** parse flags. Flag-level
  patterns (e.g. `assistant jira create --project=*`) are roadmap.
- Do **not** parse shell composition. For `FOO=bar assistant … && other`, match
  the first real command after env-prefix stripping; pipes / `&&` / subshells are
  best-effort, not guaranteed. Full shell parsing does not fit the <100ms budget.

### 3.3 Risk / breadth (§8.1)

Define command **breadth = number of fixed leading tokens before the wildcard**.
A bare-binary pattern (`assistant *`, one fixed token) is broad and escalates
risk one level — the direct analog of a path glob spanning more than one
top-level directory. Subcommand-qualified patterns (`assistant jira create *`)
do not bump.

No `sensitive_commands` escalator in v1 — there is no clean analog to the
path-based `**/migrations/**` sensitive-path globs. Deferred to roadmap.

### 3.4 Specificity ranking (§11)

More fixed leading tokens = higher specificity = ranks above broader command
matches, mirroring glob specificity. So for `assistant jira create …`, a precise
`assistant jira create *` lesson outranks a generic `assistant *` one. This falls
directly out of the breadth token-count in §3.3.

### 3.5 Scope correction (§8.5)

`adjust_scope` observations carry command patterns too. The subset relation is
clean under the token-prefix model:

```
assistant jira create *  ⊆  assistant jira *  ⊆  assistant *
```

Narrowing (suggested ⊆ current) applies immediately; broadening needs
substantiation (human `approve` or a later independent confirm matching the
broader pattern). Unchanged from path semantics.

### 3.6 Hook integration (§10.1)

- Add `Bash` to the `PreToolUse` matcher (`internal/cli/plugin.go:22`).
- Parse `tool_input.command` from the hook stdin payload
  (`internal/cli/checkaction.go` `hookInput`; today it reads only `file_path`).
- Add a `Command` field to `retrieve.Query`; the hook populates `Command` for
  Bash and `Paths` for edits.
- Still local-index-only, no network, <100ms.

**Enforcement: `requirement` blocks Bash.** An unacknowledged active
`requirement` whose `scope.commands` matches the command **denies** the Bash call
(deny + guidance + `tm ack` instruction), exactly as it denies a matching edit.
Precise structural matching earns this; it is what makes "this command needs
setup first" actually enforceable. (Edit-time blocking is unchanged.)

### 3.7 Cross-agent parity (§10.3)

`tm_check_action` (MCP) gains an optional `command` argument routed to
`Query.Command`, so agents without an edit-time hook (Codex, Cursor, Continue)
can perform the voluntary command-time check with the same matching.

### 3.8 Memory-worthiness: no system/OS-specific memories (§5.1, §10.3)

Memories are **team-shared and repo-scoped**. A fact that holds only on one
machine — an OS-divergent flag (`--foo` vs `--bar`), interpreter name
(`python` vs `python3`), path separator, or local toolchain version — is not
team project judgment; it would be **wrong for part of the team** and actively
mislead them.

- `tm_propose`'s tool description (§10.3) and the §5.1 non-examples gain an
  explicit exclusion of system/OS/host-specific facts, with the rationale stated.
- The SessionStart brief's standing instructions (§10.1) carry the same caution.
- Especially flagged for command lessons, since the matcher (§3.2) deliberately
  does not normalize OS differences.

## 4. Testing

- **Command matcher (unit):** positional token match; trailing-wildcard
  one-or-more semantics; `VAR=val` env-prefix stripping; bare-binary breadth;
  subset relation for `adjust_scope`; non-match cases (`assistant *` must not
  match `assistantd`).
- **Derived state (golden files):** command-breadth risk escalation; effective
  scope under command `adjust_scope` narrow/broaden.
- **Retrieval precision:** command channel candidate selection; provisional
  command lessons surfacing as caution at command time; ranking vs broader
  command patterns.
- **Hook:** Bash payload parsing; `requirement` deny on matching command +
  ack-then-pass; latency stays under the 100ms budget with command scopes in the
  ledger.

## 5. Roadmap deferrals

- Flag-level command patterns.
- `sensitive_commands` risk escalators.
- Robust shell-composition parsing (pipes / `&&` / subshells).

## 6. Open questions

None. (Enforcement-on-Bash resolved: allow the block.)
