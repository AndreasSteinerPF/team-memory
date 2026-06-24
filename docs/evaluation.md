# Evaluation

> This document is an explanatory projection of [`prd.md`](../prd.md). It
> separates verification that exists from evaluations that are proposed.

## What success means

TeamMemory succeeds when relevant, evidence-backed project judgment changes an
agent's behavior at the right moment without excessive false warnings, blocks,
approval work, or context cost. The central question is whether teams repeat
fewer known mistakes while retaining correction paths for bad or stale guidance
(`prd.md §§3, 14`).

Success has four dimensions:

1. **Behavioral utility:** fewer repeated project-specific mistakes.
2. **Precision:** relevant memories surface and unrelated ones stay quiet.
3. **Governance quality:** bad memories are corrected and binding rules receive
   proportionate scrutiny.
4. **Operational fitness:** hooks, sync, indexing, and adapters remain fast and
   dependable.

## Existing verification

The repository verifies mechanisms more strongly than production outcomes.

- Unit tests cover models, derivation, policy, scope/command matching,
  retrieval, ledger sync, SQLite replay, safety scanning, acknowledgments,
  nudges, CLI, MCP, and adapters.
- End-to-end tests cover proposal, observation, activation, governance,
  requirement blocking, acknowledgment, duplicate/supersede flows, export,
  background push, and two-clone convergence.
- The flagship demo drives proposal → independent confirmation → warning →
  human requirement → blocked action (`prd.md §13`).
- The trap-repo test mechanically contrasts a voluntary action check with a
  pre-tool requirement denial for a seeded pitfall. It does not run an agent or
  measure completed-task quality (`prd.md §14`).
- Performance coverage checks the synchronous hook path at 1,000 memories.
- Cross-harness contract, replay, and packaging tiers run on fixtures;
  build-tag-gated tests exercise installed CLIs (`prd.md §10.6`).
- `tm nudge report` summarizes firing, suppression, delivery, context size, and
  same-session follow-through. It is operational instrumentation, not a causal
  A/B evaluation (`prd.md §§10.1, 17`).

These checks establish specified mechanism behavior. They do not establish the
size of TeamMemory's effect across diverse production tasks.

## Evaluation scenarios

### Repeated failed-fix prevention

Seed a tempting fix that previously failed for a non-obvious reason. Compare
whether an agent repeats it before and after a scoped `failed_attempt` activates.

### Unsafe migration prevention

Give an agent a migration task requiring rollback validation. Measure whether a
warning or requirement causes the check before finalization.

### Flaky-test workaround recall

Record a project-specific precondition or known-invalid workaround. Later
sessions should retrieve it for the relevant suite without suppressing
investigation of new failures.

### Project-convention enforcement

Test a convention not cheaply inferable from source, such as a required
generation command or release sequence. Compare advisory and requirement
conditions for compliance and interruption cost.

### Multi-agent consistency

Run equivalent tasks through supported agents with the same ledger. Compare
guidance and requirement behavior while preserving adapter-specific caveats.

### Longitudinal usefulness

Review memories at increasing ages and code drift. Ask whether each remains
correct, minimally scoped, and worth surfacing.

## Suggested metrics

| Metric | Definition |
|---|---|
| Repeated mistake rate | Eligible tasks that repeat a known failure |
| Warning precision | Relevant surfaced warnings / all surfaced warnings |
| Warning recall | Relevant memories surfaced / relevant memories available |
| Block false-positive rate | Denials judged unnecessary or wrongly scoped |
| Human approval burden | Review time and approvals per useful requirement |
| Time to correction | Time until bad guidance becomes contested, stale, rejected, or superseded |
| Memory usefulness over time | Maintainer-rated utility by age and drift |
| Acknowledgment rate | Requirements bypassed without satisfying guidance |
| Context cost | Injected bytes/tokens and extra turns |
| Cross-agent consistency | Equivalent outcomes across adapters |

Precision and recall require a labeled relevance set; retrieval counts alone
can reward noise.

## Suggested manual protocol

1. Build small repositories containing one realistic, non-obvious trap.
2. Define safe behavior and relevance labels before agent runs.
3. Compare baseline and TeamMemory conditions, randomizing order where possible.
4. Use fresh sessions and comparable model/tool settings; retain transcripts,
   diffs, commands, hook decisions, and test results.
5. Have reviewers blinded to condition score mistake repetition, relevance,
   interruption, and solution quality.
6. Repeat across models, harnesses, and task variants; report caveats rather
   than pooling incompatible runs.
7. Review whether each memory was correct, scoped, evidenced, and useful.

## Suggested automated harness

Build on the trap-repo and cross-harness infrastructure:

- encode each scenario as a repository, prompt, trap, relevant-memory set, and
  observable success criteria;
- run baseline, advisory, and requirement conditions;
- capture retrieval, hook decisions, commands, edits, acknowledgments, final
  diffs, tests, and context size;
- normalize volatile harness fields while preserving evidence;
- score deterministic outcomes automatically and use blinded review for
  semantic quality;
- keep fixture contracts in normal CI and gate costly live/model runs.

This comparative harness is proposed. The repository contains foundations and
individual scenarios, not the full evaluation system.

## Limitations and open questions

- Outcomes vary with model, prompt, permissions, and harness version.
- Seeded traps can overfit wording.
- Preventing one side effect does not prove a better final solution.
- Relevance labels and approval-burden measurements are subjective.
- Long-term utility and behavior with a large noisy ledger require field data.
- Security evaluation needs dedicated adversarial scenarios.

Open questions include the precision needed to preserve trust, the point where
approval cost exceeds blocking value, the effect of confirmation on accuracy,
appropriate decay/expiry policy, and how much benefit comes from retrieval
versus governance and action-time delivery.
