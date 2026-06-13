# Critical-tier Auto-Activation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `critical`-risk memories auto-activate at 2 independent confirms (capped at `warning` enforcement) instead of requiring a human `approve`, while keeping `requirement` human-only.

**Architecture:** Add an optional per-tier `min_independent_confirms` threshold to the policy. The `independent_confirm` activation mode reads it (omitted ⇒ 1, so medium/high are unchanged); the critical tier sets it to 2. Derivation, enforcement, docs, and tests move together — `prd.md` updates in the same commit per `AGENTS.md`.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, standard `testing`.

**Spec:** `docs/superpowers/specs/2026-06-13-critical-auto-activation-design.md`

---

## File Structure

- `internal/policy/policy.go` — add `MinIndependentConfirms` to `Tier`; set critical default. (Modify)
- `internal/derive/status.go` — `isActive()` reads the threshold. (Modify)
- `internal/derive/status_test.go` — rewrite `TestCriticalNeedsHumanApprove` for the new behavior. (Modify)
- `prd.md` — §8.1 policy YAML block, §8.2 activation rule. (Modify)
- `README.md` — Policy excerpt + governance note. (Modify)

---

### Task 1: Policy — add `MinIndependentConfirms` field and critical default

**Files:**
- Modify: `internal/policy/policy.go` (`Tier` struct ~line 35; `Default()` critical tier ~line 82)
- Test: `internal/policy/policy_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/policy/policy_test.go`:

```go
func TestDefaultCriticalTierAutoActivates(t *testing.T) {
	tier := policy.Default().Activation.Tiers[model.RiskCritical]
	if tier.Auto != "independent_confirm" {
		t.Errorf("critical auto = %q, want independent_confirm", tier.Auto)
	}
	if tier.MinIndependentConfirms != 2 {
		t.Errorf("critical min_independent_confirms = %d, want 2", tier.MinIndependentConfirms)
	}
	if tier.MaxAutoEnforcement != model.EnforcementWarning {
		t.Errorf("critical max_auto_enforcement = %q, want warning", tier.MaxAutoEnforcement)
	}
}

func TestDefaultYAMLRoundTrips(t *testing.T) {
	y, err := policy.DefaultYAML()
	if err != nil {
		t.Fatal(err)
	}
	got, err := policy.Load(y)
	if err != nil {
		t.Fatal(err)
	}
	if got.Activation.Tiers[model.RiskCritical].MinIndependentConfirms != 2 {
		t.Errorf("round-trip lost min_independent_confirms: %+v", got.Activation.Tiers[model.RiskCritical])
	}
}
```

Ensure the test file imports `"github.com/AndreasSteinerPF/team-memory/internal/model"` and `"github.com/AndreasSteinerPF/team-memory/internal/policy"` (add if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/policy/ -run 'TestDefaultCriticalTierAutoActivates|TestDefaultYAMLRoundTrips' -v`
Expected: FAIL — `tier.MinIndependentConfirms` undefined (field doesn't exist yet), or critical `Auto` is `"never"`.

- [ ] **Step 3: Add the struct field**

In `internal/policy/policy.go`, change the `Tier` struct:

```go
type Tier struct {
	Auto                   string            `yaml:"auto"` // immediate | independent_confirm | never
	MinIndependentConfirms int               `yaml:"min_independent_confirms,omitempty"`
	MaxAutoEnforcement     model.Enforcement `yaml:"max_auto_enforcement,omitempty"`
}
```

- [ ] **Step 4: Update the critical default**

In `Default()`, replace the critical tier line:

```go
model.RiskCritical: {Auto: "never"},
```

with:

```go
model.RiskCritical: {Auto: "independent_confirm", MinIndependentConfirms: 2, MaxAutoEnforcement: model.EnforcementWarning},
```

Leave low/medium/high unchanged (they omit `MinIndependentConfirms`, which defaults to 0 → treated as 1 by the derive logic in Task 2).

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/policy/ -v`
Expected: PASS (including the existing round-trip/default tests).

- [ ] **Step 6: Commit**

```bash
git add internal/policy/policy.go internal/policy/policy_test.go
git commit -m "feat(policy): per-tier min_independent_confirms; critical=2"
```

---

### Task 2: Derivation — `isActive()` honors the threshold

**Files:**
- Modify: `internal/derive/status.go` (`isActive()`, lines 69-78)
- Test: `internal/derive/status_test.go` (rewrite `TestCriticalNeedsHumanApprove`, lines 84-97)

- [ ] **Step 1: Rewrite the failing test**

Replace `TestCriticalNeedsHumanApprove` in `internal/derive/status_test.go` with:

```go
func TestCriticalAutoActivatesOnTwoConfirms(t *testing.T) {
	p := policy.Default()
	m := model.Memory{Type: model.TypeConstraint, Actor: model.Actor{SessionID: "s1"}, CreatedAt: ts(0)}

	// One independent confirm is not enough for critical (bar is 2).
	c1 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s2"}, CreatedAt: ts(1)}
	st, _ := computeStatus(m, []model.Observation{c1}, model.RiskCritical, p)
	if st != model.StatusProvisional {
		t.Errorf("critical with one confirm → %q, want provisional", st)
	}

	// A second independent confirm activates it.
	c2 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s3"}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{c1, c2}, model.RiskCritical, p)
	if st != model.StatusActive {
		t.Errorf("critical with two independent confirms → %q, want active", st)
	}

	// A human approve still activates instantly, regardless of confirm count.
	approve := model.Observation{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{approve}, model.RiskCritical, p)
	if st != model.StatusActive {
		t.Errorf("critical with human approve → %q, want active", st)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/ -run TestCriticalAutoActivatesOnTwoConfirms -v`
Expected: FAIL on the two-confirms case → got `provisional`, want `active` (current `auto: "never"` only activates on human approve).

- [ ] **Step 3: Update `isActive()`**

In `internal/derive/status.go`, replace the `independent_confirm` case:

```go
func isActive(obs []model.Observation, risk model.Risk, indConf int, p policy.Policy) bool {
	switch p.Activation.Tiers[risk].Auto {
	case "immediate":
		return true
	case "independent_confirm":
		threshold := p.Activation.Tiers[risk].MinIndependentConfirms
		if threshold < 1 {
			threshold = 1 // omitted/0 ⇒ 1 (back-compat for medium/high)
		}
		return indConf >= threshold || existsHumanApprove(obs)
	default: // "never" or unknown
		return existsHumanApprove(obs)
	}
}
```

- [ ] **Step 4: Run the derive tests to verify they pass**

Run: `go test ./internal/derive/ -v`
Expected: PASS — `TestCriticalAutoActivatesOnTwoConfirms` passes, and `TestStatusProgression` (high → active on **one** confirm) still passes, confirming medium/high are unaffected.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/status.go internal/derive/status_test.go
git commit -m "feat(derive): critical activates at 2 independent confirms"
```

---

### Task 3: Docs — prd.md and README (same-commit spec sync)

**Files:**
- Modify: `prd.md` (§8.1 policy YAML block; §8.2 activation rule)
- Modify: `README.md` (Policy excerpt; governance note)

- [ ] **Step 1: Update prd.md §8.1 policy YAML block**

Find the activation tiers block (the `low: / medium: / high: / critical:` lines) and replace the critical line:

```
    critical: { auto: never }              # human approve only
```

with:

```
    critical: { auto: independent_confirm, min_independent_confirms: 2, max_auto_enforcement: warning }  # 2 confirms; requirement still human-only
```

- [ ] **Step 2: Update prd.md §8.2 activation rule**

Find:

```
   * `critical`: active only with a human `approve`.
```

Replace with:

```
   * `critical`: active once ≥2 *independent* `confirm`s exist, or a human `approve` exists.
```

- [ ] **Step 3: Update README.md Policy excerpt**

Find the critical block in the `activation:` excerpt:

```
    critical:                             # never auto-activates; needs a human
      auto: never
```

Replace with:

```
    critical:                             # two independent confirms activate it
      auto: independent_confirm
      min_independent_confirms: 2
      max_auto_enforcement: warning
```

- [ ] **Step 4: Update README.md governance note**

Find:

```
`critical` tiers never auto-activate, and no tier can reach `requirement` without `tm approve` — agents alone can never create a binding rule.
```

Replace with:

```
`critical` memories need two independent confirmations to auto-activate — more evidence than any other tier — and no tier can reach `requirement` without `tm approve`, so agents alone can never create a binding rule.
```

- [ ] **Step 5: Verify the whole suite is green**

Run: `go test ./... -count=1`
Expected: all packages `ok`.

- [ ] **Step 6: Commit**

```bash
git add prd.md README.md
git commit -m "docs: critical auto-activates at 2 confirms (prd.md §8 + README)"
```

---

## Self-Review

**Spec coverage:**
- Per-tier confirm threshold field → Task 1. ✓
- Critical default `{independent_confirm, 2, warning}` → Task 1. ✓
- `isActive()` threshold logic (omitted ⇒ 1) → Task 2. ✓
- `requirement` stays human-only → unchanged `enforcement.go`; verified by the full suite in Task 3 Step 5. ✓
- prd.md §8.1 + §8.2 → Task 3 Steps 1-2. ✓
- README policy excerpt + governance note → Task 3 Steps 3-4. ✓
- Round-trip invariant → Task 1 `TestDefaultYAMLRoundTrips`. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. ✓

**Type consistency:** `MinIndependentConfirms int` used identically in policy struct, `Default()`, derive `isActive()`, and both tests. `model.EnforcementWarning`, `model.RiskCritical`, `model.KindConfirm/KindApprove`, `computeStatus`, `existsHumanApprove`, `ts()` all match existing definitions. ✓

**Note:** Medium/high regression guard is `TestStatusProgression` (high → active on 1 confirm), already covered in Task 2 Step 4 — no separate task needed.
