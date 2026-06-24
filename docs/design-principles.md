# Design principles

> This document is an explanatory projection of [`prd.md`](../prd.md), the
> authoritative specification. It describes current implementation and
> tradeoffs; it does not define behavior independently (`prd.md §6`).

## Project memory should be reviewable and auditable

**Why it matters.** Agent guidance can influence work long after its originating
session. Teams need to inspect who recorded a claim, its evidence, and the
observations that changed its state.

**Implemented today.** Memories and observations are immutable YAML records on
the orphan `teammemory` branch. Approvals, contradictions, stale marks,
duplicate links, and supersession are appended rather than applied as in-place
updates. State can be replayed from records and policy (`prd.md §§5.3–5.4,
7–9`).

**Tradeoffs and limitations.** Git provides history and familiar review, not
tamper resistance. Append-only storage also makes accidental sensitive-data
retention difficult to repair.

## Agents should not unilaterally create hard requirements

**Why it matters.** One mistaken or compromised agent should not turn its
interpretation into binding project policy.

**Implemented today.** Agent proposals may auto-activate only at advisory
levels. Only a human `approve` observation can set `requirement`, and MCP
exposes no approval tool (`prd.md §§5.6, 8.4, 10.3`).

**Tradeoffs and limitations.** TeamMemory does not authenticate human identity
or authorization. Repository and machine access controls remain external.

## Memory should be scoped, not global by default

**Why it matters.** A migration lesson or command prerequisite is useful only
where it applies. Global advice increases false positives and spreads
machine-specific trivia.

**Implemented today.** Memories carry repository-relative path globs and/or
token-aware command patterns. Scope narrowing applies immediately; widening
requires evidence or human approval (`prd.md §§8.5, 9.1, 11`).

**Tradeoffs and limitations.** The scope languages favor predictability over
expressiveness and cannot represent every symbol-level or shell relationship.

## Warnings and hard blocks are separate lifecycle stages

**Why it matters.** Evidence worth surfacing is not automatically strong enough
to halt work.

**Implemented today.** Status, confidence, and enforcement are separate derived
dimensions. Confirmation can activate a recommendation or warning; requirement
blocking needs human promotion. Session-local acknowledgment permits deliberate
continuation without weakening shared state (`prd.md §§5.5–5.6, 8.2–8.4,
10.2`).

**Tradeoffs and limitations.** More states add conceptual overhead.
Acknowledgment is an escape hatch, so the system reduces risk rather than
guaranteeing compliance.

## Binding rules require human approval

**Why it matters.** Blocking policy affects developer autonomy and delivery and
should remain visible and accountable.

**Implemented today.** `tm approve` appends a human observation and can set
confidence and enforcement; `tm reject` is human-only and terminal
(`prd.md §§8.2–8.4, 10.5`).

**Tradeoffs and limitations.** There is no signature verification,
multi-maintainer quorum, delegated role model, or approval impact preview.

## Integrate with existing developer workflows

**Why it matters.** Project memory is useful at the point of action and must be
inspectable with tools engineers already use.

**Implemented today.** TeamMemory uses Git for storage, a Go CLI for operators,
MCP for voluntary agent actions, per-harness hooks for delivery, and local
SQLite for network-free checks (`prd.md §§7, 10, 16`).

**Tradeoffs and limitations.** Agent hook APIs change independently. A shared
engine reduces divergence, but adapters retain different event coverage, trust
setup, and advisory timing.

## Governance, not retrieval alone

**Why it matters.** Finding relevant text does not establish whether it is
credible, current, properly scoped, or strong enough to block.

**Implemented today.** Retrieval operates inside a lifecycle with typed
records, confirmation, contradiction, stale/duplicate/supersede states,
policy-derived risk, human promotion, and acknowledgment (`prd.md §§2, 5–8,
11`).

**Tradeoffs and limitations.** Governance adds friction and cannot determine
truth automatically. Independent confirmation can reproduce a shared
misunderstanding.

## Derived state should be reproducible

**Why it matters.** Agents should not directly set authoritative status, risk,
confidence, or enforcement.

**Implemented today.** Those values are pure functions of immutable records and
policy. SQLite stores a disposable projection that can be replayed
(`prd.md §§5.4, 7.3, 8`).

**Tradeoffs and limitations.** Policy or derivation changes can alter current
state without changing old records, so they require same-change PRD updates and
careful tests.
