# Lightweight Nudge Outcome Instrumentation Design

## Context

`prd.md` Section 17 now calls for lightweight nudge outcome instrumentation before any formal nudge A/B capability evaluation. The current nudge engine already keeps per-session local journals under `.git/tm/nudge` as diagnostic state, never as synced ledger records (`prd.md` Section 10.1). The feature should extend that local state just enough to answer operational tuning questions without introducing a telemetry subsystem.

The key product question is: are nudges firing, being delivered, being suppressed, and followed up on often enough to justify their current budget defaults?

## Goals

- Report aggregate nudge behavior for operational tuning.
- Keep all instrumentation local to `.git/tm/nudge`.
- Preserve the existing hook-safety posture: hooks stay silent on failure where they already do, and hook-time work remains constant-time bookkeeping.
- Compute follow-through with simple target matching where possible.
- Make the report useful from the CLI via `tm nudge report`.

## Non-Goals

- No synced telemetry or ledger schema changes.
- No JSONL event stream or separate analytics store.
- No formal A/B evaluation harness.
- No model scoring, per-agent quality score, timing analysis, date ranges, or pruning in v1.
- No MCP tool changes unless a later implementation pass finds the CLI-only report insufficient.

## Data Model

Extend `internal/nudge.Journal` with local-only outcome details.

`FiredNudge` should keep its current `Key` and `Turn` fields, and add optional metadata:

- `Type`: signal type such as `fail_pass`, `churn`, `unobserved`, or `self_review`.
- `Verb`: `propose`, `observe`, or empty for self-review nudges.
- `Path`: exact repo-relative path target when the signal has one.
- `MemoryID`: target memory id for observe nudges.
- `TextBytes`: byte length of the injected nudge text.
- `Delivery`: `rendered` or `queued`.
- `FiredAt`: timestamp when the nudge fired.
- `DrainedTurn`: turn when queued text was drained through `UserPromptSubmit`, if applicable.

Add a `Suppression` structure to the journal:

- `Reason`: `disabled`, `max_per_session`, `cooldown`, `dedup`, or `already_acted`.
- `Type`, `Verb`, `Path`, and `MemoryID`: signal metadata when suppression applies to a candidate signal.
- `Turn`: journal turn when the suppression was recorded.

`nudge.Decide` should return a structured decision result instead of only `(Nudge, bool)`, so the nudge policy code remains the source of truth for suppression reasons. The caller should not reimplement cooldown, budget, dedup, or already-acted logic.

## Hook Behavior

`tm nudge --hook` records outcome metadata when a nudge fires:

- Append a `FiredNudge` entry with signal metadata and `TextBytes`.
- Set `Delivery` to `queued` for Claude and Codex Stop hooks, because those nudges are reinjected at prompt time.
- Set `Delivery` to `rendered` for harnesses that render the Stop decision immediately.

When no nudge fires, `tm nudge --hook` remains silent but records suppression metadata when a candidate was suppressed. If there are no candidates and the only reason is "nothing to do," no suppression entry is needed.

`tm signal --hook --prompt` records queued delivery by updating matching queued `FiredNudge` entries with `DrainedTurn` when it drains `journal.Pending`.

`tm signal --hook` continues to record edits, command outcomes, surfaced memories, and injected advisory memory ids as it does today. It does not need to aggregate report data.

## Report Command

Add `tm nudge report` under the existing `nudge` command.

Default report:

- Reads local journals from `.git/tm/nudge`.
- Aggregates across all sessions.
- Prints compact human-readable counts:
  - sessions
  - turns
  - detected candidates
  - fired
  - suppressed by reason
  - rendered
  - queued
  - drained
  - follow-through: target-matched, session-level, none, unavailable

Flags:

- `--session <id>`: report one journal.
- `--json`: output the same summary in machine-readable JSON.

Corrupt journal files are skipped with a warning. If the ledger or index is unavailable, the report still prints fired, suppressed, and delivery counts, and marks follow-through as unavailable.

## Follow-Through

Follow-through is computed on demand by comparing fired nudge metadata with ledger records from the same session. Use `FiredAt` and the ledger records' `CreatedAt` timestamps for "after this nudge" checks; journal turn numbers are local to the hook journal and are not stored on ledger records.

- Observe nudges are target-matched when a same-session observation targets `MemoryID` after `FiredAt`.
- Propose nudges are target-matched when a same-session memory has an exact path overlap with `Path` after `FiredAt`.
- Targetless self-review nudges use session-level follow-through: any same-session memory or observation after `FiredAt` counts.
- If ledger records cannot be read, follow-through is unavailable rather than false.

This intentionally keeps matching simple. No fuzzy path matching, semantic matching, or quality judgment belongs in v1.

## Testing

Add focused coverage:

- Unit tests for `nudge.Decide` suppression reasons.
- Journal round-trip test for the new `FiredNudge` and `Suppression` fields.
- CLI report tests for human-readable output and `--json`.
- Follow-through tests for observe memory-id match, propose path match, targetless self-review fallback, no match, and unavailable ledger.
- Hook regression tests that `tm nudge --hook` remains silent when no nudge fires and still queues for Claude/Codex.

## Open Decisions

None. The design intentionally chooses the local journal as the only persistence surface and `tm nudge report` as the only user-facing surface for v1.
