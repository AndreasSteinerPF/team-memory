# Technical Credibility Documentation Design

## Purpose

Add an implementation-backed technical documentation layer for TeamMemory aimed
at senior engineers, AI infrastructure teams, coding-agent teams, and technical
recruiters. The documentation must explain the system precisely without
presenting planned work as shipped behavior or overstating the beta product's
safety guarantees.

The authoritative product behavior remains `prd.md`. New documents will cite
relevant sections as `prd.md §X.Y` and use the implementation and tests as
evidence that the documented behavior exists.

## Approach

Use five focused reference documents rather than a single long narrative:

- `docs/architecture.md` explains components, boundaries, lifecycle data flow,
  Git-backed storage, local indexing, integrations, and limitations.
- `docs/design-principles.md` explains the governance philosophy and gives the
  implementation status and tradeoffs of each principle.
- `docs/threat-model.md` evaluates practical failure and abuse modes using
  description, impact, current mitigation, and recommended future mitigation.
- `docs/evaluation.md` separates the repository's existing verification from
  proposed product-quality evaluation scenarios, metrics, and harnesses.
- `docs/roadmap.md` translates `prd.md §17` into an engineering-oriented view
  with implemented, near-term, medium-term, long-term, and non-goal sections.

`README.md` will receive a small technical-docs link section and factual
corrections where implementation and `prd.md` clearly contradict existing
claims. In particular, the quickstart will not list `continue` as a supported
`tm init --harness` value because `internal/cli/init.go` accepts only Claude,
Codex, Copilot, Cursor, and Gemini. Continue may still be described accurately
as compatible with Claude-style hook configuration where applicable.

## Evidence and Source Hierarchy

Claims will be checked in this order:

1. `prd.md`, especially §§5–11 and §§14–18.
2. Production packages under `internal/` and `cmd/tm`.
3. Unit, end-to-end, demo, benchmark, and harness tests.
4. Existing README, verification notes, changelog, and contributor docs.

When these disagree, the code and tests determine what is implemented, while
`prd.md` determines intended behavior. Confirmed documentation errors will be
corrected. Unresolved differences will be stated as documented discrepancies
rather than silently harmonized.

## Architecture Document

The architecture page will present TeamMemory as a repo-scoped governance and
retrieval layer for future-action-relevant project judgment.

It will describe:

- the append-only YAML memory and observation records on the orphan
  `teammemory` branch (`prd.md §§7.1–7.2`);
- deterministic derivation of status, risk, confidence, enforcement, and
  effective scope (`prd.md §§5.4, 8`);
- the disposable SQLite/FTS index under `.git/tm/` (`prd.md §7.3`);
- union-merge synchronization and separate-remote support (`prd.md §7.4`);
- CLI and five-tool MCP surfaces (`prd.md §§10.3, 10.5`);
- harness-neutral hook events with per-agent adapters (`prd.md §10.6`);
- local acknowledgments, nudge journals, and push diagnostics that do not enter
  the shared ledger;
- warning injection versus requirement blocking.

An ASCII diagram will show the record path and enforcement path. The lifecycle
will distinguish independent confirmation from human promotion: confirmation
can activate a warning according to policy, while only a human approval can
raise enforcement to `requirement`.

Known limitations will include lexical retrieval, approximate glob
intersection, adapter-specific fidelity differences, Codex multi-file patch
matching against only the first path, Git/remote operational dependencies, and
the absence of stronger identity/signature and multi-human governance.

## Design Principles Document

Each principle will use the same four-part structure:

1. Principle.
2. Why it matters.
3. Current implementation.
4. Tradeoffs and limitations.

The principles will cover reviewable/auditable memory, non-unilateral hard
requirements, scoped memory, separate advisory and blocking stages, human
authority for binding rules, integration with Git and agent workflows, and the
distinction between governance and generic retrieval.

The document will avoid treating Git history as tamper-proof or independent
confirmation as proof of truth. It will describe them as auditability and
confidence-building mechanisms with explicit limits.

## Threat Model Document

The threat model will define assets and trust boundaries before enumerating
risks. It will use a table or repeated structured sections with:

- description;
- potential impact;
- implemented mitigation, if any;
- residual risk and recommended future work.

The required risks will be covered, including false authority, single-agent
poisoning, stale or over-broad rules, prompt injection, sensitive-data
retention, branch/context conflicts, over-reliance, and warning fatigue.

Current mitigations will be limited to what exists, such as derived activation
gates, human-only requirement promotion, contradiction/stale/duplicate/
supersede observations, scope and risk policy, proposal-time regex safety
scanning, bounded retrieval, local acknowledgments, and nudge budgets.

The document will explicitly note gaps: regex scanning is incomplete, agent
identity may degrade to session identity, Git authorization is external to
TeamMemory, malicious content can still influence agent-authored proposals, and
there is no cryptographic signing or multi-party approval.

## Evaluation Document

The evaluation page will separate:

### Existing verification

- unit tests for derivation, policy, retrieval, storage, safety, adapters, and
  CLI behavior;
- end-to-end lifecycle and enforcement tests;
- the flagship demo;
- trap-repo mistake-avoidance coverage;
- two-clone union-merge convergence;
- hook latency coverage;
- cross-harness contract, replay, packaging, and gated live tests;
- local nudge outcome reporting, described as diagnostics rather than formal
  causal evaluation.

### Proposed evaluation

- repeated failed-fix prevention;
- unsafe migration prevention;
- flaky-test workaround recall;
- convention enforcement;
- multi-agent consistency;
- longitudinal memory utility and decay.

Metrics will include mistake recurrence, warning precision/recall, block false
positives, approval burden, correction time, usefulness over time, and
operational/context cost. The manual protocol will use blinded baseline versus
TeamMemory runs where practical. The automated harness proposal will build on
the existing trap-repo and cross-harness infrastructure without claiming it
already exists.

## Roadmap Document

The roadmap will derive from `prd.md §17` and will not attach dates.

- Current status will summarize shipped phases and beta constraints.
- Near-term priorities will describe the GitHub/PR workflow in the greatest
  detail, including rationale, dependencies, tradeoffs, and spec references.
- Medium-term priorities will cover retrieval depth and additional harnesses.
- Long-term questions will cover governance depth, stronger identity, review
  UX, interoperability, expiry, and research questions.
- Non-goals will reflect `prd.md §§4, 12.3`.

Detail will intentionally decrease with distance from the current phase, in
line with the project memory governing roadmap presentation.

## README Changes

Keep changes narrow:

- add a “Technical docs” section linking all five documents;
- remove `continue` from the `tm init --harness` quickstart option list;
- correct any additional factual mismatch found during final cross-check only
  when implementation and `prd.md` provide unambiguous evidence.

No feature implementation or broad README rewrite is in scope.

## Verification and Review

Verification will include:

- link/path checks for all new README links and internal relative links;
- searches for unsupported absolute claims and ambiguous future tense;
- Markdown cleanliness checks available in the repository/environment;
- `go test ./...` and `go build ./...`, because documentation corrections may
  expose references tied to tested command or harness behavior;
- an independent code/documentation review with explicit `APPROVED`;
- a separate spec-compliance review with explicit `APPROVED`.

Any review finding will be corrected and the revised final changes reviewed
again. The temporary files under `docs/superpowers/` will be removed before the
completed work is delivered.
