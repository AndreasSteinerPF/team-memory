# Threat model

> This document is an explanatory projection of [`prd.md`](../prd.md). It
> describes risks in the current beta implementation; it is not a security
> guarantee.

## Scope and trust boundaries

Protected assets include the integrity of shared guidance, availability of
normal developer actions, confidentiality of memory content, and auditability
of governance decisions. Trust crosses agents and model context, human
operators, Git authorization, third-party hook APIs, local `.git/tm/` state,
and potentially malicious project content.

TeamMemory assumes the local binary and repository are not already fully
compromised. It does not sandbox agents, authenticate humans, sign records, or
replace Git-host access control (`prd.md §§4, 12.3, 15`).

## Incorrect memories become authoritative

- **Description:** A plausible but false lesson is confirmed and promoted.
- **Potential impact:** Agents avoid valid approaches or are blocked from safe
  work.
- **Implemented mitigation:** Activation gates, contradiction-to-contested
  transitions, stale/duplicate/supersede/reject workflows, and human-only
  requirement promotion (`prd.md §§8.2–8.4`).
- **Residual risk / future work:** Confirmation is evidence, not proof. Add
  richer review, expiry/revalidation, and warning-precision evaluation.

## A single agent poisons shared memory

- **Description:** One agent records malicious, noisy, or biased guidance.
- **Potential impact:** Warning fatigue, retrieval bias, or later policy capture.
- **Implemented mitigation:** Risk-tier gates, a confirmation gate for
  `successful_pattern`, bounded provisional retrieval, duplicate warnings, and
  human-only requirements (`prd.md §§8.1–8.2, 11, 15`).
- **Residual risk / future work:** Low-risk types can activate immediately and
  session IDs are weak identity. Signed records, stronger actors, quality
  controls, and multi-party promotion are not implemented.

## Stale memories block valid work

- **Description:** Code or external constraints change while a requirement
  remains active.
- **Potential impact:** Correct actions are denied and users learn to bypass the
  system.
- **Implemented mitigation:** Anchor-drift annotation, `mark_stale`,
  contradiction, supersession, stale-candidate listing, and local
  acknowledgment (`prd.md §§8.2, 8.6, 10.2`).
- **Residual risk / future work:** Drift is coarse and cannot prove semantic
  invalidation. Add expiry and scheduled revalidation.

## Requirements are scoped too broadly

- **Description:** A valid lesson applies to unrelated paths or commands.
- **Potential impact:** False-positive warnings or blocks.
- **Implemented mitigation:** Explicit scopes, broad-scope risk escalation,
  sensitive-path floors, immediate narrowing, and substantiated widening
  (`prd.md §§8.1, 8.5`).
- **Residual risk / future work:** Matching is approximate and humans can approve
  broad rules. Promotion should preview affected files and commands.

## Prompt injection or malicious project content influences memory

- **Description:** Source, logs, issues, or tool output induce an agent to
  persist misleading instructions.
- **Potential impact:** Poisoned memory, indirect instruction persistence, or
  leakage into future contexts.
- **Implemented mitigation:** Agent guidance limits memory-worthy content;
  proposals remain subject to lifecycle gates and human-only requirement
  promotion (`prd.md §§5.1, 10.1, 10.3, 15`).
- **Residual risk / future work:** Memory guidance is free-form and is injected
  into agent context; TeamMemory does not enforce descriptive wording, classify
  provenance, or prove evidence supports a proposal. Add untrusted-content
  labeling, content-policy checks, and adversarial evaluation.

## Sensitive information is stored

- **Description:** A proposal contains a credential, identifier, or
  confidential value that append-only Git replicates.
- **Potential impact:** Persistent disclosure in clones, remotes, and backups.
- **Implemented mitigation:** Proposal-time regex scanning blocks selected
  secret patterns and warns on selected PII by default (`prd.md §§15, 17`).
- **Residual risk / future work:** Regex coverage is incomplete and fallible.
  Add broader scanners, redaction UX, and a history-rewrite incident procedure.

## Branch- or context-specific lessons conflict

- **Description:** Branches or environments record different valid behavior.
- **Potential impact:** Contested state or the wrong lesson after merge.
- **Implemented mitigation:** Optional branch/path/command context,
  contradiction, scope adjustment, duplicate links, and supersession preserve
  evidence and history (`prd.md §§8.2, 8.5, 9`).
- **Residual risk / future work:** Retrieval is repository-scoped rather than
  branch-conditioned. Context-specific effective state remains open.

## Agents over-rely on remembered constraints

- **Description:** An agent treats guidance as truth and stops checking current
  code or tests.
- **Potential impact:** Cargo-cult fixes and missed invalidation.
- **Implemented mitigation:** Provisional framing, visible state/reason/evidence,
  drift annotations, and separate advisory/requirement stages (`prd.md
  §§5.5–5.6, 11`).
- **Residual risk / future work:** Model behavior is not deterministic. Evaluate
  wording and expose verification evidence more directly.

## Developers become fatigued by noisy warnings

- **Description:** Weakly relevant messages cause users to ignore TeamMemory.
- **Potential impact:** Useful warnings lose credibility and approvals or
  acknowledgments become reflexive.
- **Implemented mitigation:** Precision-first ranking, result caps, provisional
  caps, drift down-ranking, advisory limits, and nudge budgets (`prd.md §§10.1,
  11, 15`).
- **Residual risk / future work:** Diagnostics do not establish usefulness.
  Measure precision, acknowledgment behavior, and longitudinal value.

## Git or local-state tampering

- **Description:** An authorized user or compromised process rewrites the
  ledger, policy, index, or acknowledgment files.
- **Potential impact:** Disappearing memories, altered enforcement, or bypassed
  blocks.
- **Implemented mitigation:** Ordinary Git history is inspectable, the index is
  replayable, and acknowledgments are intentionally session-local.
- **Residual risk / future work:** There are no signatures or tamper-evident
  acks. Use protected remotes and least privilege today; investigate signed
  governance later.

## Third-party hook behavior changes

- **Description:** An agent CLI changes events, payloads, trust setup, or output
  handling.
- **Potential impact:** Silent warning loss or failed blocking.
- **Implemented mitigation:** Thin adapters, committed contract fixtures,
  replay/packaging tests, capability conformance, and gated live verification
  (`prd.md §10.6`).
- **Residual risk / future work:** Not all live behavior fits deterministic CI.
  Maintain compatibility notes, scheduled live checks, and fail-visible
  diagnostics.
