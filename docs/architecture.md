# Architecture

> This document is an explanatory projection of the authoritative
> [`prd.md`](../prd.md). If they differ, `prd.md` governs intended behavior;
> code and tests determine what is implemented.

## System purpose

TeamMemory is a local-first, repository-scoped governance and retrieval layer
for project knowledge that should affect future agent actions. It records
immutable memories and observations, derives their current state from policy,
and surfaces relevant guidance around coding-agent actions. Validated memories
may warn; only human approval can promote one to a blocking requirement
(`prd.md §§5–8`).

It is not a transcript store or general semantic-memory service. Its unit is
future-action-relevant project judgment: a failed approach, hidden constraint,
fragile area, stale document, undocumented decision, or repeatedly successful
pattern (`prd.md §5.2`).

## Components

| Component | Responsibility |
|---|---|
| Git ledger | Immutable YAML memories, observations, and `policy.yaml` on the orphan `teammemory` branch |
| Derivation engine | Pure computation of status, risk, confidence, enforcement, and effective scope |
| Local index | Disposable SQLite/FTS5 materialization under `.git/tm/index.db` |
| CLI | Initialization, governance, inspection, synchronization, and hook entry points |
| MCP server | Agent-facing proposal, observation, action check, search, and status tools |
| Hook engine | Harness-neutral events and advisory/deny decisions |
| Agent adapters | Claude Code, Codex, Copilot CLI, Cursor, and Gemini CLI wire formats |
| Local session state | Acknowledgments, nudge journals, fetch timestamps, and push diagnostics under `.git/tm/` |

The ledger is authoritative shared state. SQLite is a rebuildable cache, and
session-local files are operational state rather than shared memory
(`prd.md §§7.1–7.4, 10.1–10.3`).

## Lifecycle and data flow

```text
agent / CLI
    |
    | propose
    v
immutable YAML record ---> orphan Git branch ---> background push
    |                            |
    |                            | fetch + union-merge
    v                            v
derive state from records + policy
    |
    v
SQLite/FTS index
    |
    +--> inspection and lexical search
    |
    +--> action query (path, command, text)
              |
        rank matching memories
              |
       +------+----------------+
       |                       |
 advisory context       unacknowledged requirement
                               |
                         deny tool call
                               |
                    session-local acknowledgment
```

### Proposal

`tm propose` and `tm_propose` create typed memory envelopes. The CLI and MCP
surfaces first run configurable regex-based secret and PII checks. The record
receives a ULID and is committed as `memories/<id>.yaml` through Git plumbing,
without checking out the ledger branch or changing the normal Git index
(`prd.md §§7.1–7.2, 9.1`).

Status and enforcement are derived, not mutable fields supplied by the agent.
Lexically similar memories may produce a duplicate warning, but this is
advisory.

### Independent confirmation and activation

An agent that independently encounters the lesson can append a `confirm`
observation. The default independence mode requires a different session;
optional modes also consider branch or Git email. Missing identity metadata can
degrade stricter modes to session-only checks (`prd.md §§8.2, 9.1`).

Risk comes from memory type, scope, and policy. Low-risk memories can activate
immediately; medium and high risk require one independent confirmation by
default; critical risk requires two. Automatic enforcement is capped below
`requirement` (`prd.md §§8.1–8.4`). Confirmation raises confidence; it does not
prove that a claim is universally true.

Contradictions, stale marks, duplicate links, supersession, approvals, and
rejection participate in a deterministic precedence model. Scope narrowing
applies immediately; broadening requires substantiation (`prd.md §§8.2, 8.5`).

### Human promotion and enforcement

`tm approve <id> --enforcement requirement` appends a human observation. MCP
does not expose approval. TeamMemory structurally separates this command from
agent actions, but it does not cryptographically authenticate a human or their
authority (`prd.md §§5.6, 8.4, 10.3, 10.5`).

Hook adapters parse edits and commands into neutral queries. Retrieval combines
path globs, command patterns, and lexical text; excludes stale, rejected,
duplicate, and superseded memories; optionally surfaces structurally related
provisional or contested memories as cautions; applies result caps; and ranks
precise structural matches first (`prd.md §11`). Advisories inject context. An
unacknowledged requirement produces a deny on harnesses with a pre-tool blocking
surface. Acknowledgments are local to the session and do not change the shared
rule (`prd.md §§10.1–10.2`).

## Git-backed branch model

The orphan `teammemory` branch contains `policy.yaml`, `memories/`, and
`observations/`, not project source. Commits are built with a private temporary
index. Each append uses a unique ULID filename, so concurrent writers normally
add different paths (`prd.md §§7.1–7.2`).

On divergence, `tm sync` creates a merge commit whose tree unions both record
sets; local `policy.yaml` wins if that shared path collides. Lost push races are
retried a bounded number of times. Proposals and observations push
best-effort in the background, while action checks can initiate a non-blocking
background fetch. The synchronous hook path remains local. A separate ledger
remote handles repositories whose source remote rejects the orphan branch
(`prd.md §§7.1, 7.4`).

## Why local-first and repository-scoped

- Project judgment stays near the code and uses familiar Git review, access,
  replication, and backup workflows.
- Action checks query a local cache without requiring a hosted service or
  network round trip.
- Explicit repository, path, and command scope reduces cross-project and
  machine-specific contamination.
- Ordinary Git tools expose record and governance history.

Git improves reviewability; it is not tamper-proof. Anyone able to rewrite or
delete the branch can alter its history.

## Integration boundaries

The CLI and MCP server share ledger, derivation, indexing, and retrieval
packages. MCP exposes `tm_propose`, `tm_observe`, `tm_check_action`,
`tm_search`, and `tm_status`; approval and rejection remain CLI-only
(`prd.md §§10.3, 10.5`).

The hook engine is common, but delivery fidelity depends on third-party agent
APIs (`prd.md §10.6`). Advisory timing differs; Claude Code and Codex currently
do not expose failed commands to post-tool hooks; Cursor lacks pre-edit
blocking. See [Cross-harness configuration](harnesses.md).

## Limitations and open questions

- Retrieval is lexical and structural, without embeddings or semantic
  equivalence (`prd.md §§11, 12.3`).
- Glob intersection and containment are conservative approximations.
- Codex multi-file `apply_patch` calls match only the first extracted path
  because the neutral event carries one path.
- Session and Git-email identity are signals, not authenticated identities.
- Regex sensitive-data scanning can miss secrets or produce false positives.
- Sync depends on Git credentials, branch protection, and remote availability.
- Append-only history makes accidental secret retention costly to remediate.
- Governance has no signatures, multi-human quorum, or expiry workflow.
- Real-world, long-term mistake reduction remains an evaluation question, not
  an established product claim (`prd.md §§12.3, 17`).
