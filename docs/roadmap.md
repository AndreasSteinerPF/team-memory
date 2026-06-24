# Roadmap

> This roadmap is an explanatory projection of the authoritative
> [`prd.md §17`](../prd.md#17-roadmap). It carries no dates or commitments.
> Priority changes should land in `prd.md` first.

## Current status

TeamMemory is beta software with the core governance loop implemented:
Git-backed append-only records, deterministic state, policy-driven activation,
local lexical retrieval, CLI and five-tool MCP surfaces, human-only requirement
promotion, acknowledgment, sync, diagnostics, and adapters for Claude Code,
Codex, Copilot CLI, Cursor, and Gemini CLI (`prd.md §§7–11, 17`).

The repository includes unit and end-to-end coverage, a flagship demo,
trap-repo benchmark, performance checks, and cross-harness contract and live
verification. Proposal-time sensitive-data scanning, optional actor-aware
confirmation, and local nudge diagnostics are also shipped.

Beta constraints remain: lexical retrieval, weak identity, Git and hook-API
dependencies, CLI-heavy review, and limited evidence of long-term production
mistake reduction.

## Near-term priorities

### Put memory on the pull-request review surface

The next major surface in `prd.md §17` is a GitHub workflow that evaluates
changed paths and relevant commands, then reports applicable memories as a
check or comment.

This makes project judgment visible when an active agent did not surface it and
creates a familiar place to discuss correctness, scope, approval, rejection,
and staleness. Engineering questions include comment update strategy,
permissions, fork safety, noise control, deterministic rendering, and whether a
future version should ever block merging. The first version should remain
advisory unless the normative spec defines stronger behavior.

### Add a memory timeline and review view

A static view can render proposal and observation history, current state,
pending scope/supersession claims, contradictions, stale candidates, and the
human action behind a requirement (`prd.md §§12.2, 17`). It must remain a
projection of the Git ledger, not a second mutable authority.

## Medium-term priorities

- **Retrieval depth:** symbol anchors, error-signature matching, content-hash
  drift, and evaluated semantic ranking (`prd.md §17`).
- **Agent breadth:** OpenCode, Pi, and other agents with stable hook/MCP surfaces.

## Long-term research and product questions

- Signed records, multi-human approval, policy templates, expiry workflows, and
  contested-memory review UI (`prd.md §17`).
- Proactive verification by reviewer agents.
- Semantic retrieval without opaque or noisy ranking.
- Durable team-outcome measurement rather than benchmark compliance.

## Candidate investments not yet on the authoritative roadmap

The following are reasonable engineering directions requested for consideration,
but they are not committed priorities in `prd.md §17`. Promoting any of them to
roadmap status requires a same-change update to the PRD:

- better documentation, worked examples, and executable documentation checks;
- a comparative evaluation harness for precision, false blocks, approval
  burden, and context cost;
- improved conflict handling and contested-memory review;
- safer promotion UX with scope and impact previews;
- better retrieval, suppression, adapter, and sync observability;
- richer memory review UX;
- import/export interoperability that preserves provenance, scope, validation,
  and enforcement semantics. Existing `tm export` is not a general
  bidirectional interchange protocol.

## Non-goals

Unless `prd.md` changes, TeamMemory is not intended to become:

- a hosted transcript or personal-memory service;
- a replacement for docs, tests, code review, or Git hosting;
- an autonomous authority for project truth;
- a general vector database;
- a guarantee that agents obey guidance or requirements are safe;
- a store for every fact, event, or session summary;
- a place to persist secrets or personal data.

These boundaries follow `prd.md §§4, 12.3`.
