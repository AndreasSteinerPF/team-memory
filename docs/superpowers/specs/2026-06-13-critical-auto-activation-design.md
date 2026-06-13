# Critical-tier auto-activation

**Date:** 2026-06-13
**Status:** approved, ready for implementation

## Problem

`critical`-risk memories currently never auto-activate. Per the default policy
(`internal/policy/policy.go`, `prd.md §8.1`), the critical tier is
`{ auto: never }`, so a critical memory stays `provisional` — invisible to
active retrieval and the `PreToolUse` hook — until a human runs `tm approve`.

Under the default sensitive-path escalators, "critical" covers `**/auth/**` and
`.github/workflows/**`. Requiring a human to activate *every* memory in those
areas is heavier governance than wanted: agents should be able to validate
critical memories among themselves, with a higher evidence bar than lower tiers,
while humans retain the gate on hard enforcement.

## Goal

Allow `critical` memories to auto-activate on sufficient independent evidence,
without weakening the human gate on the edit-blocking `requirement` level.

Two decisions (made during brainstorming):

1. **Evidence bar:** 2 independent confirms (stricter than `high`, which needs 1).
2. **Auto-enforcement ceiling:** `warning` — the same ceiling as `high`.
   `requirement` remains reachable only via human `approve`.

## Non-goals

- No change to `requirement` enforcement: it stays human-only
  (`requirement_enforcement.human_required: true`, hardcoded in
  `internal/derive/enforcement.go`). Critical can auto-reach at most `warning`.
- No change to stale/reject semantics.
- No change to medium/high/low tiers.

## Design

### New mechanism: per-tier confirm threshold

The existing `independent_confirm` activation mode is hardcoded to activate at
`indConf >= 1` (`internal/derive/status.go`). To require more evidence for
critical without inventing a new mode, add an optional per-tier confirm count.

**`internal/policy/policy.go`**

- Add a field to `Tier`:
  ```go
  MinIndependentConfirms int `yaml:"min_independent_confirms,omitempty"`
  ```
- `Default()`: change the critical tier to
  ```go
  model.RiskCritical: {
      Auto:               "independent_confirm",
      MinIndependentConfirms: 2,
      MaxAutoEnforcement: model.EnforcementWarning,
  },
  ```
- Medium/high/low are unchanged. They omit `MinIndependentConfirms`; an omitted
  (zero) value is treated as 1, preserving their current bar.
- `DefaultYAML()` serializes the new field automatically; the
  `Load(DefaultYAML()) == Default()` round-trip invariant must still hold.

### Derivation change

**`internal/derive/status.go`** — the `independent_confirm` case of `isActive()`:

```go
case "independent_confirm":
    threshold := p.Activation.Tiers[risk].MinIndependentConfirms
    if threshold < 1 {
        threshold = 1 // omitted/0 ⇒ 1 (back-compat for medium/high)
    }
    return indConf >= threshold || existsHumanApprove(obs)
```

`existsHumanApprove` short-circuits regardless of the count, so a human `approve`
still activates a critical memory instantly (unchanged).

No change to `internal/derive/enforcement.go`: an active critical memory picks up
its tier's `max_auto_enforcement` (`warning`), and `requirement` stays gated.

### Resulting behavior

| Critical memory state | Status | Enforcement |
|---|---|---|
| Proposed, 0 confirms | provisional | hint |
| 1 independent confirm | provisional | hint |
| 2 independent confirms | active | warning |
| Human `approve` (any time) | active | up to `requirement` if set |

## Docs to update in the same commit (AGENTS.md rule)

- **`prd.md`**:
  - §8.1 policy YAML block — critical tier line.
  - §8.2 activation rule — `critical: active only with a human approve` becomes
    `critical: active once ≥2 independent confirms, or a human approve`.
- **`README.md`**:
  - Policy excerpt — replace `critical: { auto: never }`.
  - Governance/lifecycle note about human involvement (critical no longer
    *requires* a human to activate; human still required for `requirement`).

## Tests

**`internal/derive/status_test.go`**
- critical + 1 independent confirm → `provisional` (new).
- critical + 2 independent confirms → `active` (new).
- critical + human `approve` → `active` (existing — keep passing).

**`internal/policy`** (existing round-trip test)
- `Load(DefaultYAML()) == Default()` still holds with the new field.

**`internal/derive/enforcement_test.go`** (optional)
- active critical → `warning`.

## Risks

- **Solo dogfooding:** independent confirms require distinct sessions
  (`activation.independence: different_session`). A 2-confirm bar means critical
  memories are the hardest to auto-activate when working alone; the human
  `approve` path remains the practical route during solo use. Acceptable —
  matches the "critical needs the most evidence" intent.
