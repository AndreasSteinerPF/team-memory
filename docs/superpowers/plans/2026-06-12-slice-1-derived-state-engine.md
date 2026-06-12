# Slice 1: Domain Model + Derived-State Engine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the pure-Go domain model and the `Derive(memory, observations, policy) → DerivedState` function that computes status, risk, confidence, enforcement, and effective scope exactly per `prd.md` §8.

**Architecture:** Three internal packages with no I/O. `model` holds record types and enums. `policy` holds the policy struct, defaults, and YAML loading. `derive` holds the pure derivation function, split one-concern-per-file (risk, scope, status, confidence, enforcement) behind a single `Derive` orchestrator. Correctness is pinned by table tests for the glob/risk helpers and golden YAML scenarios for the full function — including the flagship-demo lifecycle.

**Tech Stack:** Go 1.26, `gopkg.in/yaml.v3`, standard library `testing`. No git, no SQLite, no third-party runtime deps beyond yaml.

---

## File structure

```text
go.mod
.gitignore
internal/model/model.go            # all record types + enums
internal/model/model_test.go
internal/policy/policy.go          # Policy struct, Default(), Load()
internal/policy/policy_test.go
internal/derive/helpers.go         # shared obs filters/counters
internal/derive/scope.go           # glob predicates + effective scope
internal/derive/scope_test.go
internal/derive/risk.go            # risk ranks + riskForScope
internal/derive/risk_test.go
internal/derive/status.go          # independence + status
internal/derive/confidence.go      # confidence
internal/derive/enforcement.go     # enforcement
internal/derive/derive.go          # DerivedState + Derive orchestrator + reason
internal/derive/derive_test.go     # golden lifecycle scenarios
internal/derive/testdata/*.yaml    # golden fixtures
```

`internal/` package import paths are `github.com/AndreasSteinerPF/team-memory/internal/<pkg>`.

---

### Task 0: Project scaffolding

**Files:**
- Create: `go.mod`, `.gitignore`

- [ ] **Step 1: Initialize the repo and module**

Run:
```bash
cd /c/Users/andys/team-memory
git init
go mod init github.com/AndreasSteinerPF/team-memory
go mod edit -go=1.26
go get gopkg.in/yaml.v3@latest
```
Expected: `go.mod` created with module `github.com/AndreasSteinerPF/team-memory`, `go 1.26`, and a `require gopkg.in/yaml.v3` line; `go.sum` populated.

- [ ] **Step 2: Write `.gitignore`**

Create `.gitignore`:
```gitignore
# build output
/tm
/tm.exe
/dist/

# go
*.test
*.out

# local memory index (created by the tool at runtime, never committed)
.git/tm/
```

- [ ] **Step 3: Verify the toolchain builds an empty module**

Run: `go build ./...`
Expected: exits 0, no output (no packages yet).

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: scaffold tm Go module"
```

---

### Task 1: Domain model types

**Files:**
- Create: `internal/model/model.go`
- Test: `internal/model/model_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Create `internal/model/model_test.go`:
```go
package model

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestMemoryYAMLRoundTrip(t *testing.T) {
	in := Memory{
		ID:      "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Type:    TypeFailedAttempt,
		Title:   "Billing migrations require downgrade-path tests",
		Summary: "rollback failed",
		Scope:   Scope{Paths: []string{"billing/migrations/**"}},
		CodeContext: &CodeContext{
			Branch: "feature/invoice-state",
			Commit: "abc123def",
		},
		Actor:     Actor{Kind: ActorAgent, Name: "claude-code", SessionID: "session_123"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Memory
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != in.ID || out.Type != in.Type ||
		out.CodeContext == nil || out.CodeContext.Branch != "feature/invoice-state" ||
		len(out.Scope.Paths) != 1 || out.Scope.Paths[0] != "billing/migrations/**" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestObservationCarriesKindFields(t *testing.T) {
	o := Observation{
		ID:             "01J8X5A2P4HND7QW9XK1MZRTGE",
		Target:         "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Kind:           KindAdjustScope,
		SuggestedScope: &Scope{Paths: []string{"billing/migrations/manual/**"}},
		Actor:          Actor{Kind: ActorAgent, Name: "codex", SessionID: "session_456"},
		CreatedAt:      time.Now(),
	}
	data, _ := yaml.Marshal(o)
	var out Observation
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SuggestedScope == nil || out.SuggestedScope.Paths[0] != "billing/migrations/manual/**" {
		t.Fatalf("suggested_scope lost: %+v", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/...`
Expected: FAIL — `undefined: Memory` (and the other identifiers).

- [ ] **Step 3: Write `internal/model/model.go`**

```go
// Package model defines TeamMemory's record types and the small set of
// enumerations used to describe them. It contains no logic and no I/O.
package model

import "time"

type MemoryType string

const (
	TypeFailedAttempt MemoryType = "failed_attempt"
	TypeConstraint    MemoryType = "constraint"
	TypeFragileArea   MemoryType = "fragile_area"
	TypeStaleDoc      MemoryType = "stale_doc"
	TypeDecision      MemoryType = "decision"
)

type ConstraintOrigin string

const (
	OriginTeam     ConstraintOrigin = "team"
	OriginExternal ConstraintOrigin = "external"
)

type ObservationKind string

const (
	KindConfirm     ObservationKind = "confirm"
	KindContradict  ObservationKind = "contradict"
	KindAdjustScope ObservationKind = "adjust_scope"
	KindMarkStale   ObservationKind = "mark_stale"
	KindApprove     ObservationKind = "approve"
	KindReject      ObservationKind = "reject"
)

type ActorKind string

const (
	ActorAgent ActorKind = "agent"
	ActorHuman ActorKind = "human"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type Enforcement string

const (
	EnforcementHint           Enforcement = "hint"
	EnforcementRecommendation Enforcement = "recommendation"
	EnforcementWarning        Enforcement = "warning"
	EnforcementRequirement    Enforcement = "requirement"
)

type Status string

const (
	StatusProvisional Status = "provisional"
	StatusActive      Status = "active"
	StatusContested   Status = "contested"
	StatusStale       Status = "stale"
	StatusRejected    Status = "rejected"
)

// Scope is a set of path globs the memory applies to.
type Scope struct {
	Paths []string `yaml:"paths"`
}

// Actor identifies who created a record.
type Actor struct {
	Kind      ActorKind `yaml:"kind"`
	Name      string    `yaml:"name"`
	SessionID string    `yaml:"session_id,omitempty"`
}

// CodeContext records where work happened. On a memory it is where the memory
// was proposed; on an observation it is where the observing agent was working.
type CodeContext struct {
	Branch string   `yaml:"branch,omitempty"`
	Commit string   `yaml:"commit,omitempty"`
	Paths  []string `yaml:"paths,omitempty"`
}

// Evidence is a pointer to something that substantiates a record.
type Evidence struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description,omitempty"`
	Ref         string `yaml:"ref,omitempty"`
}

// Anchor ties a memory to a path at a commit.
type Anchor struct {
	Path   string `yaml:"path"`
	Commit string `yaml:"commit"`
}

// Memory is the immutable envelope. Status, risk, confidence, enforcement, and
// effective scope are NOT stored here — they are derived (see package derive).
type Memory struct {
	ID          string           `yaml:"id"`
	Type        MemoryType       `yaml:"type"`
	Origin      ConstraintOrigin `yaml:"origin,omitempty"` // only for type=constraint
	Title       string           `yaml:"title"`
	Summary     string           `yaml:"summary,omitempty"`
	Guidance    string           `yaml:"guidance,omitempty"`
	Scope       Scope            `yaml:"scope"`
	Evidence    []Evidence       `yaml:"evidence,omitempty"`
	Anchors     []Anchor         `yaml:"anchors,omitempty"`
	CodeContext *CodeContext     `yaml:"code_context,omitempty"`
	Actor       Actor            `yaml:"actor"`
	CreatedAt   time.Time        `yaml:"created_at"`
}

// Observation is an immutable reaction to a memory.
type Observation struct {
	ID             string          `yaml:"id"`
	Target         string          `yaml:"target"`
	Kind           ObservationKind `yaml:"kind"`
	Summary        string          `yaml:"summary,omitempty"`
	Evidence       []Evidence      `yaml:"evidence,omitempty"`
	CodeContext    *CodeContext    `yaml:"code_context,omitempty"`
	SuggestedScope *Scope          `yaml:"suggested_scope,omitempty"` // kind=adjust_scope
	SetEnforcement Enforcement     `yaml:"set_enforcement,omitempty"` // kind=approve
	SetConfidence  Confidence      `yaml:"set_confidence,omitempty"`  // kind=approve
	Actor          Actor           `yaml:"actor"`
	CreatedAt      time.Time       `yaml:"created_at"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/...`
Expected: PASS (`ok  github.com/AndreasSteinerPF/team-memory/internal/model`).

- [ ] **Step 5: Commit**

```bash
git add internal/model/
git commit -m "feat(model): domain record types and enums"
```

---

### Task 2: Policy types, defaults, and loading

**Files:**
- Create: `internal/policy/policy.go`
- Test: `internal/policy/policy_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/policy/policy_test.go`:
```go
package policy

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestDefaultPolicyMatchesPRD(t *testing.T) {
	p := Default()

	if p.BaseRisk[model.TypeFailedAttempt] != model.RiskMedium {
		t.Errorf("failed_attempt base risk = %q, want medium", p.BaseRisk[model.TypeFailedAttempt])
	}
	if p.BaseRisk[model.TypeStaleDoc] != model.RiskLow {
		t.Errorf("stale_doc base risk = %q, want low", p.BaseRisk[model.TypeStaleDoc])
	}
	if !p.Escalators.BroadScopeBump {
		t.Error("broad_scope_bump should default true")
	}
	if p.Activation.Independence != "different_session" {
		t.Errorf("independence = %q, want different_session", p.Activation.Independence)
	}
	if p.Activation.Tiers[model.RiskLow].Auto != "immediate" {
		t.Errorf("low tier auto = %q, want immediate", p.Activation.Tiers[model.RiskLow].Auto)
	}
	if p.Activation.Tiers[model.RiskCritical].Auto != "never" {
		t.Errorf("critical tier auto = %q, want never", p.Activation.Tiers[model.RiskCritical].Auto)
	}
	if p.Activation.Tiers[model.RiskHigh].MaxAutoEnforcement != model.EnforcementWarning {
		t.Errorf("high tier max enforcement = %q, want warning", p.Activation.Tiers[model.RiskHigh].MaxAutoEnforcement)
	}
	if len(p.Escalators.SensitivePaths) == 0 {
		t.Error("expected default sensitive paths")
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	yml := []byte(`
base_risk:
  failed_attempt: high
activation:
  independence: different_session_and_branch
`)
	p, err := Load(yml)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.BaseRisk[model.TypeFailedAttempt] != model.RiskHigh {
		t.Errorf("override base risk = %q, want high", p.BaseRisk[model.TypeFailedAttempt])
	}
	// Unspecified keys fall back to defaults.
	if p.BaseRisk[model.TypeStaleDoc] != model.RiskLow {
		t.Errorf("merged stale_doc = %q, want low", p.BaseRisk[model.TypeStaleDoc])
	}
	if p.Activation.Independence != "different_session_and_branch" {
		t.Errorf("independence = %q", p.Activation.Independence)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/policy/...`
Expected: FAIL — `undefined: Default` / `undefined: Load`.

- [ ] **Step 3: Write `internal/policy/policy.go`**

```go
// Package policy defines the configurable policy that drives state derivation,
// its built-in defaults (matching prd.md §8.1), and YAML loading that merges
// user overrides onto those defaults.
package policy

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"gopkg.in/yaml.v3"
)

type Policy struct {
	BaseRisk    map[model.MemoryType]model.Risk `yaml:"base_risk"`
	Escalators  Escalators                      `yaml:"escalators"`
	Activation  Activation                      `yaml:"activation"`
	Retrieval   Retrieval                       `yaml:"retrieval"`
	Sync        Sync                            `yaml:"sync"`
}

type Escalators struct {
	BroadScopeBump bool            `yaml:"broad_scope_bump"`
	SensitivePaths []SensitivePath `yaml:"sensitive_paths"`
}

type SensitivePath struct {
	Glob    string     `yaml:"glob"`
	MinRisk model.Risk `yaml:"min_risk"`
}

type Activation struct {
	Independence string                    `yaml:"independence"` // different_session | different_session_and_branch
	Tiers        map[model.Risk]Tier       `yaml:"tiers"`
}

type Tier struct {
	Auto               string            `yaml:"auto"` // immediate | independent_confirm | never
	MaxAutoEnforcement model.Enforcement `yaml:"max_auto_enforcement,omitempty"`
}

type Retrieval struct {
	MaxResults      int    `yaml:"max_results"`
	MaxProvisional  int    `yaml:"max_provisional"`
	ProvisionalMode string `yaml:"provisional_mode"` // never | related | always
}

type Sync struct {
	AutoFetchAfter string `yaml:"auto_fetch_after"` // duration string, e.g. "5m"
}

// Default returns the built-in policy from prd.md §8.1.
func Default() Policy {
	return Policy{
		BaseRisk: map[model.MemoryType]model.Risk{
			model.TypeStaleDoc:      model.RiskLow,
			model.TypeDecision:      model.RiskLow,
			model.TypeFailedAttempt: model.RiskMedium,
			model.TypeFragileArea:   model.RiskMedium,
			model.TypeConstraint:    model.RiskMedium, // origin=external escalates to high in derive
		},
		Escalators: Escalators{
			BroadScopeBump: true,
			SensitivePaths: []SensitivePath{
				{Glob: "**/migrations/**", MinRisk: model.RiskHigh},
				{Glob: "**/auth/**", MinRisk: model.RiskCritical},
				{Glob: ".github/workflows/**", MinRisk: model.RiskCritical},
			},
		},
		Activation: Activation{
			Independence: "different_session",
			Tiers: map[model.Risk]Tier{
				model.RiskLow:      {Auto: "immediate", MaxAutoEnforcement: model.EnforcementRecommendation},
				model.RiskMedium:   {Auto: "independent_confirm", MaxAutoEnforcement: model.EnforcementWarning},
				model.RiskHigh:     {Auto: "independent_confirm", MaxAutoEnforcement: model.EnforcementWarning},
				model.RiskCritical: {Auto: "never"},
			},
		},
		Retrieval: Retrieval{MaxResults: 5, MaxProvisional: 2, ProvisionalMode: "related"},
		Sync:      Sync{AutoFetchAfter: "5m"},
	}
}

// Load parses YAML over a copy of Default(), so unspecified keys keep their
// default values. Note: yaml.Unmarshal replaces whole maps/slices it sees, so
// a user who specifies base_risk replaces only the keys they list — see merge
// of scalar sub-fields below.
func Load(data []byte) (Policy, error) {
	p := Default()
	// Merge base_risk and tiers key-by-key rather than wholesale-replacing the
	// default maps. Decode into a sparse overlay first.
	var overlay Policy
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return Policy{}, err
	}
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Policy{}, err
	}
	// yaml.Unmarshal merges into existing maps (adds/overrides keys, keeps
	// others), so p.BaseRisk and p.Activation.Tiers retain unspecified defaults.
	// overlay is unused beyond validating the document parses; kept for clarity.
	_ = overlay
	return p, nil
}
```

> Implementation note for the engineer: `gopkg.in/yaml.v3` merges decoded keys **into** an existing non-nil map rather than replacing it, which is exactly the behaviour `TestLoadOverridesDefaults` asserts (override `failed_attempt`, keep `stale_doc`). The double-unmarshal/overlay is therefore redundant — if the test passes with a single `yaml.Unmarshal(data, &p)`, simplify to that. Keep whichever form makes the test green; do not add a manual merge loop unless the test fails.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/policy/...`
Expected: PASS. If `TestLoadOverridesDefaults` fails because the default map was replaced wholesale, simplify `Load` to a single `yaml.Unmarshal(data, &p)` per the note and re-run.

- [ ] **Step 5: Commit**

```bash
git add internal/policy/
git commit -m "feat(policy): policy struct, defaults, and YAML loading"
```

---

### Task 3: Scope glob predicates

**Files:**
- Create: `internal/derive/scope.go`
- Test: `internal/derive/scope_test.go`

These pure helpers are the trickiest correctness surface. They implement deliberately simple, segment-based rules (documented limitations below) — full glob-algebra is a roadmap item.

- [ ] **Step 1: Write the failing tests**

Create `internal/derive/scope_test.go`:
```go
package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestScopeIsBroad(t *testing.T) {
	cases := []struct {
		glob string
		want bool
	}{
		{"billing/migrations/**", false},
		{"**", true},
		{"*/**", true},
		{"src/**", false},
		{"**/auth/**", true},
	}
	for _, c := range cases {
		if got := globIsBroad(c.glob); got != c.want {
			t.Errorf("globIsBroad(%q) = %v, want %v", c.glob, got, c.want)
		}
	}
}

func TestGlobIntersects(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"billing/migrations/**", "**/migrations/**", true},
		{"billing/migrations/**", "**/auth/**", false},
		{"docs/migrations-guide/**", "**/migrations/**", false}, // segment-exact, no false match
		{"**", "**/auth/**", true},                               // ** touches everything
		{".github/workflows/ci.yml", ".github/workflows/**", true},
	}
	for _, c := range cases {
		if got := globIntersects(c.a, c.b); got != c.want {
			t.Errorf("globIntersects(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestScopeSubset(t *testing.T) {
	outer := model.Scope{Paths: []string{"billing/migrations/**"}}
	narrower := model.Scope{Paths: []string{"billing/migrations/manual/**"}}
	broader := model.Scope{Paths: []string{"billing/**"}}

	if !scopeSubset(narrower, outer) {
		t.Error("manual/** should be subset of migrations/**")
	}
	if scopeSubset(broader, outer) {
		t.Error("billing/** should NOT be subset of migrations/**")
	}
}

func TestPathMatchesScope(t *testing.T) {
	s := model.Scope{Paths: []string{"billing/migrations/**"}}
	if !pathMatchesScope("billing/migrations/2026_add_invoice_state.sql", s) {
		t.Error("path under migrations should match")
	}
	if pathMatchesScope("billing/reports/q1.go", s) {
		t.Error("path outside migrations should not match")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/...`
Expected: FAIL — `undefined: globIsBroad` etc.

- [ ] **Step 3: Write `internal/derive/scope.go`**

```go
package derive

import (
	"strings"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// segments splits a glob into path segments, trimming slashes.
func segments(glob string) []string {
	glob = strings.Trim(glob, "/")
	if glob == "" {
		return nil
	}
	return strings.Split(glob, "/")
}

func hasWildcard(seg string) bool {
	return strings.ContainsAny(seg, "*?[")
}

// matchesEverything reports whether the glob is the catch-all "**".
func matchesEverything(glob string) bool {
	s := segments(glob)
	return len(s) == 1 && s[0] == "**"
}

// globIsBroad: the glob's first segment is itself a wildcard, so it can match
// paths across more than one top-level directory.
func globIsBroad(glob string) bool {
	s := segments(glob)
	if len(s) == 0 {
		return true
	}
	return hasWildcard(s[0])
}

func scopeIsBroad(s model.Scope) bool {
	for _, g := range s.Paths {
		if globIsBroad(g) {
			return true
		}
	}
	return false
}

// literalSegments returns the non-wildcard segments of a glob, in order.
func literalSegments(glob string) []string {
	var out []string
	for _, s := range segments(glob) {
		if !hasWildcard(s) {
			out = append(out, s)
		}
	}
	return out
}

// orderedSubsequence reports whether all of sub appear in seq in order.
func orderedSubsequence(sub, seq []string) bool {
	i := 0
	for _, s := range seq {
		if i < len(sub) && s == sub[i] {
			i++
		}
	}
	return i == len(sub)
}

// globIntersects: two globs intersect if either is the catch-all, or one's
// literal segments are an ordered subsequence of the other's. Simple and
// segment-exact (no partial-token matching). Full glob intersection is roadmap.
func globIntersects(a, b string) bool {
	if matchesEverything(a) || matchesEverything(b) {
		return true
	}
	la, lb := literalSegments(a), literalSegments(b)
	return orderedSubsequence(la, lb) || orderedSubsequence(lb, la)
}

// literalPrefix returns the leading non-wildcard segments of a glob.
func literalPrefix(glob string) []string {
	var out []string
	for _, s := range segments(glob) {
		if hasWildcard(s) {
			break
		}
		out = append(out, s)
	}
	return out
}

// globContains: does outer contain inner (inner ⊆ outer)? True when outer is
// the catch-all, or outer's literal prefix is a segment-prefix of inner.
func globContains(outer, inner string) bool {
	if matchesEverything(outer) {
		return true
	}
	lp := literalPrefix(outer)
	ins := segments(inner)
	if len(lp) > len(ins) {
		return false
	}
	for i, seg := range lp {
		if ins[i] != seg {
			return false
		}
	}
	return true
}

// scopeSubset: every glob in inner is contained by some glob in outer.
func scopeSubset(inner, outer model.Scope) bool {
	for _, ig := range inner.Paths {
		ok := false
		for _, og := range outer.Paths {
			if globContains(og, ig) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// pathMatchesGlob matches a concrete path against a glob. "**" matches the rest;
// a single-segment wildcard matches exactly one segment.
func pathMatchesGlob(path, glob string) bool {
	if matchesEverything(glob) {
		return true
	}
	gsegs := segments(glob)
	psegs := segments(path)
	for i, g := range gsegs {
		if g == "**" {
			return true
		}
		if i >= len(psegs) {
			return false
		}
		if hasWildcard(g) {
			continue
		}
		if psegs[i] != g {
			return false
		}
	}
	return len(psegs) == len(gsegs)
}

func pathMatchesScope(path string, s model.Scope) bool {
	for _, g := range s.Paths {
		if pathMatchesGlob(path, g) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/derive/... -run 'TestScope|TestGlob|TestPath'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/scope.go internal/derive/scope_test.go
git commit -m "feat(derive): scope glob predicates"
```

---

### Task 4: Shared observation helpers + effective scope

**Files:**
- Create: `internal/derive/helpers.go`
- Modify: `internal/derive/scope.go` (append `effectiveScope` + substantiation)
- Test: `internal/derive/scope_test.go` (append)

- [ ] **Step 1: Write the failing tests (append to `scope_test.go`)**

```go
func TestEffectiveScopeNarrowsImmediately(t *testing.T) {
	m := model.Memory{
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: tm(0),
	}
	obs := []model.Observation{{
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Paths: []string{"billing/migrations/manual/**"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      tm(1),
	}}
	got := effectiveScope(m, obs)
	if len(got.Paths) != 1 || got.Paths[0] != "billing/migrations/manual/**" {
		t.Fatalf("narrowing should apply immediately, got %v", got.Paths)
	}
}

func TestEffectiveScopeBroadeningNeedsSubstantiation(t *testing.T) {
	m := model.Memory{
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: tm(0),
	}
	adjust := model.Observation{
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Paths: []string{"billing/**"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      tm(1),
	}

	// Unsubstantiated broadening does not apply.
	if got := effectiveScope(m, []model.Observation{adjust}); got.Paths[0] != "billing/migrations/**" {
		t.Errorf("unsubstantiated broadening should NOT apply, got %v", got.Paths)
	}

	// A later independent confirm touching the broadened-but-not-prior area substantiates it.
	confirm := model.Observation{
		Kind:        model.KindConfirm,
		Actor:       model.Actor{SessionID: "s3"},
		CodeContext: &model.CodeContext{Paths: []string{"billing/reports/q1.go"}},
		CreatedAt:   tm(2),
	}
	if got := effectiveScope(m, []model.Observation{adjust, confirm}); got.Paths[0] != "billing/**" {
		t.Errorf("substantiated broadening should apply, got %v", got.Paths)
	}
}
```

Add this test helper at the top of `scope_test.go` (after the import block):
```go
func tm(sec int) time.Time { return time.Date(2026, 6, 15, 10, 0, sec, 0, time.UTC) }
```
and add `"time"` to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/... -run TestEffectiveScope`
Expected: FAIL — `undefined: effectiveScope`.

- [ ] **Step 3: Write `internal/derive/helpers.go`**

```go
package derive

import (
	"sort"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func filterKind(obs []model.Observation, kind model.ObservationKind) []model.Observation {
	var out []model.Observation
	for _, o := range obs {
		if o.Kind == kind {
			out = append(out, o)
		}
	}
	return out
}

func countKind(obs []model.Observation, kind model.ObservationKind) int {
	return len(filterKind(obs, kind))
}

func existsKind(obs []model.Observation, kind model.ObservationKind) bool {
	return countKind(obs, kind) > 0
}

func existsHumanApprove(obs []model.Observation) bool {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.Actor.Kind == model.ActorHuman {
			return true
		}
	}
	return false
}

// sortedByTime returns obs ordered by CreatedAt, then ID, ascending.
func sortedByTime(obs []model.Observation) []model.Observation {
	out := make([]model.Observation, len(obs))
	copy(out, obs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}
```

- [ ] **Step 4: Append `effectiveScope` and substantiation to `internal/derive/scope.go`**

```go
// effectiveScope applies adjust_scope observations in chronological order.
// Narrowings apply immediately; broadenings apply only once substantiated.
func effectiveScope(m model.Memory, obs []model.Observation) model.Scope {
	cur := m.Scope
	for _, a := range sortedByTime(filterKind(obs, model.KindAdjustScope)) {
		if a.SuggestedScope == nil {
			continue
		}
		sug := *a.SuggestedScope
		if scopeSubset(sug, cur) { // narrowing
			cur = sug
			continue
		}
		if broadeningSubstantiated(a, m, obs) {
			cur = sug
		}
	}
	return cur
}

// broadeningSubstantiated implements prd.md §8.5(a)/(b): a human approve after
// the adjustment, or a later independent confirm whose code-context paths fall
// inside the suggested scope but outside the original scope.
func broadeningSubstantiated(a model.Observation, m model.Memory, obs []model.Observation) bool {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.Actor.Kind == model.ActorHuman && o.CreatedAt.After(a.CreatedAt) {
			return true
		}
	}
	sug := *a.SuggestedScope
	prior := m.Scope
	for _, o := range obs {
		if o.Kind != model.KindConfirm || !o.CreatedAt.After(a.CreatedAt) {
			continue
		}
		if !isIndependent(o, m, "different_session") || o.CodeContext == nil {
			continue
		}
		matchSug, matchPrior := false, false
		for _, p := range o.CodeContext.Paths {
			if pathMatchesScope(p, sug) {
				matchSug = true
			}
			if pathMatchesScope(p, prior) {
				matchPrior = true
			}
		}
		if matchSug && !matchPrior {
			return true
		}
	}
	return false
}
```

> Note: `broadeningSubstantiated` calls `isIndependent`, defined in Task 5 (`status.go`). The package will not compile until Task 5 lands. That is expected — run this task's tests only after Task 5's `status.go` exists. If you prefer green-at-every-task, do Task 5 Step 3 (define `isIndependent`) before running tests here.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/helpers.go internal/derive/scope.go internal/derive/scope_test.go
git commit -m "feat(derive): observation helpers and effective-scope computation"
```

---

### Task 5: Independence and status

**Files:**
- Create: `internal/derive/status.go`
- Test: `internal/derive/status_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/derive/status_test.go`:
```go
package derive

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func ts(sec int) time.Time { return time.Date(2026, 6, 15, 10, 0, sec, 0, time.UTC) }

func TestIndependence(t *testing.T) {
	m := model.Memory{Actor: model.Actor{SessionID: "s1"}}
	same := model.Observation{Actor: model.Actor{SessionID: "s1"}}
	diff := model.Observation{Actor: model.Actor{SessionID: "s2"}}
	none := model.Observation{Actor: model.Actor{SessionID: ""}}

	if isIndependent(same, m, "different_session") {
		t.Error("same session is not independent")
	}
	if !isIndependent(diff, m, "different_session") {
		t.Error("different session is independent")
	}
	if isIndependent(none, m, "different_session") {
		t.Error("missing session id is not independent")
	}
}

func TestStatusProgression(t *testing.T) {
	p := policy.Default()
	// High-risk memory (migrations) — needs one independent confirm to activate.
	m := model.Memory{
		Type:      model.TypeFailedAttempt,
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{Kind: model.ActorAgent, SessionID: "s1"},
		CreatedAt: ts(0),
	}

	// No observations → provisional.
	st, _ := computeStatus(m, nil, model.RiskHigh, p)
	if st != model.StatusProvisional {
		t.Errorf("no obs → %q, want provisional", st)
	}

	// One independent confirm → active.
	confirm := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s2"}, CreatedAt: ts(1)}
	st, _ = computeStatus(m, []model.Observation{confirm}, model.RiskHigh, p)
	if st != model.StatusActive {
		t.Errorf("one independent confirm → %q, want active", st)
	}

	// Unresolved contradiction → contested.
	contra := model.Observation{Kind: model.KindContradict, Actor: model.Actor{SessionID: "s3"}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{confirm, contra}, model.RiskHigh, p)
	if st != model.StatusContested {
		t.Errorf("unresolved contradiction → %q, want contested", st)
	}

	// A newer confirm resolves the contradiction → active again.
	confirm2 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s4"}, CreatedAt: ts(3)}
	st, _ = computeStatus(m, []model.Observation{confirm, contra, confirm2}, model.RiskHigh, p)
	if st != model.StatusActive {
		t.Errorf("resolved contradiction → %q, want active", st)
	}

	// Reject is terminal.
	reject := model.Observation{Kind: model.KindReject, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(4)}
	st, _ = computeStatus(m, []model.Observation{confirm, reject}, model.RiskHigh, p)
	if st != model.StatusRejected {
		t.Errorf("reject → %q, want rejected", st)
	}
}

func TestLowRiskActivatesImmediately(t *testing.T) {
	p := policy.Default()
	m := model.Memory{Type: model.TypeStaleDoc, Actor: model.Actor{SessionID: "s1"}, CreatedAt: ts(0)}
	st, _ := computeStatus(m, nil, model.RiskLow, p)
	if st != model.StatusActive {
		t.Errorf("low risk, no obs → %q, want active", st)
	}
}

func TestCriticalNeedsHumanApprove(t *testing.T) {
	p := policy.Default()
	m := model.Memory{Type: model.TypeConstraint, Actor: model.Actor{SessionID: "s1"}, CreatedAt: ts(0)}
	confirm := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s2"}, CreatedAt: ts(1)}
	st, _ := computeStatus(m, []model.Observation{confirm}, model.RiskCritical, p)
	if st != model.StatusProvisional {
		t.Errorf("critical with only a confirm → %q, want provisional", st)
	}
	approve := model.Observation{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{confirm, approve}, model.RiskCritical, p)
	if st != model.StatusActive {
		t.Errorf("critical with human approve → %q, want active", st)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/... -run 'TestIndependence|TestStatus|TestLowRisk|TestCritical'`
Expected: FAIL — `undefined: isIndependent` / `undefined: computeStatus`.

- [ ] **Step 3: Write `internal/derive/status.go`**

```go
package derive

import (
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// isIndependent reports whether an observation counts as independent of the
// memory's proposer, per prd.md §8.2.
func isIndependent(o model.Observation, m model.Memory, mode string) bool {
	if o.Actor.SessionID == "" || o.Actor.SessionID == m.Actor.SessionID {
		return false
	}
	if mode == "different_session_and_branch" {
		mb, ob := "", ""
		if m.CodeContext != nil {
			mb = m.CodeContext.Branch
		}
		if o.CodeContext != nil {
			ob = o.CodeContext.Branch
		}
		if mb == "" || ob == "" {
			return true // degrade to session-only
		}
		return ob != mb
	}
	return true
}

func countIndependentConfirms(m model.Memory, obs []model.Observation, mode string) int {
	n := 0
	for _, o := range obs {
		if o.Kind == model.KindConfirm && isIndependent(o, m, mode) {
			n++
		}
	}
	return n
}

// resolved reports whether a confirm or approve exists strictly newer than t.
func resolved(obs []model.Observation, t time.Time) bool {
	for _, o := range obs {
		if (o.Kind == model.KindConfirm || o.Kind == model.KindApprove) && o.CreatedAt.After(t) {
			return true
		}
	}
	return false
}

// unresolved reports whether the latest observation of kind has no newer
// confirm/approve resolving it.
func unresolved(obs []model.Observation, kind model.ObservationKind) bool {
	var latest time.Time
	found := false
	for _, o := range obs {
		if o.Kind == kind && (!found || o.CreatedAt.After(latest)) {
			latest = o.CreatedAt
			found = true
		}
	}
	if !found {
		return false
	}
	return !resolved(obs, latest)
}

func isActive(obs []model.Observation, risk model.Risk, indConf int, p policy.Policy) bool {
	switch p.Activation.Tiers[risk].Auto {
	case "immediate":
		return true
	case "independent_confirm":
		return indConf >= 1 || existsHumanApprove(obs)
	default: // "never" or unknown
		return existsHumanApprove(obs)
	}
}

// computeStatus implements the precedence ladder of prd.md §8.2 and returns the
// status plus the independent-confirm count (reused by confidence).
func computeStatus(m model.Memory, obs []model.Observation, risk model.Risk, p policy.Policy) (model.Status, int) {
	indConf := countIndependentConfirms(m, obs, p.Activation.Independence)
	switch {
	case existsKind(obs, model.KindReject):
		return model.StatusRejected, indConf
	case unresolved(obs, model.KindMarkStale):
		return model.StatusStale, indConf
	case unresolved(obs, model.KindContradict):
		return model.StatusContested, indConf
	case isActive(obs, risk, indConf, p):
		return model.StatusActive, indConf
	default:
		return model.StatusProvisional, indConf
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/derive/...`
Expected: PASS (Task 4's effective-scope tests now compile and pass too, since `isIndependent` exists).

- [ ] **Step 5: Commit**

```bash
git add internal/derive/status.go internal/derive/status_test.go
git commit -m "feat(derive): independence and status computation"
```

---

### Task 6: Confidence

**Files:**
- Create: `internal/derive/confidence.go`
- Test: `internal/derive/confidence_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/derive/confidence_test.go`:
```go
package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestConfidence(t *testing.T) {
	confirm := func(sec int) model.Observation {
		return model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "x"}, CreatedAt: ts(sec)}
	}
	contra := func(sec int) model.Observation {
		return model.Observation{Kind: model.KindContradict, Actor: model.Actor{SessionID: "y"}, CreatedAt: ts(sec)}
	}
	approve := model.Observation{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(9)}

	cases := []struct {
		name    string
		obs     []model.Observation
		indConf int
		want    model.Confidence
	}{
		{"none", nil, 0, model.ConfidenceLow},
		{"one confirm", []model.Observation{confirm(1)}, 1, model.ConfidenceMedium},
		{"two confirms", []model.Observation{confirm(1), confirm(2)}, 2, model.ConfidenceHigh},
		{"human approve", []model.Observation{approve}, 0, model.ConfidenceHigh},
		{"medium minus contradiction", []model.Observation{confirm(1), contra(2)}, 1, model.ConfidenceLow},
		{"explicit set", []model.Observation{{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, SetConfidence: model.ConfidenceMedium, CreatedAt: ts(1)}}, 0, model.ConfidenceMedium},
	}
	for _, c := range cases {
		if got := computeConfidence(c.obs, c.indConf); got != c.want {
			t.Errorf("%s: confidence = %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/... -run TestConfidence`
Expected: FAIL — `undefined: computeConfidence`.

- [ ] **Step 3: Write `internal/derive/confidence.go`**

```go
package derive

import "github.com/AndreasSteinerPF/team-memory/internal/model"

var confRank = map[model.Confidence]int{
	model.ConfidenceLow: 0, model.ConfidenceMedium: 1, model.ConfidenceHigh: 2,
}
var rankConf = []model.Confidence{model.ConfidenceLow, model.ConfidenceMedium, model.ConfidenceHigh}

func approveSetConfidence(obs []model.Observation) model.Confidence {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.SetConfidence != "" {
			return o.SetConfidence
		}
	}
	return ""
}

// computeConfidence implements prd.md §8.3: base level from independent
// confirms or human approval, optional explicit override, then one step down
// per contradiction (resolved or not), floored at low.
func computeConfidence(obs []model.Observation, indConf int) model.Confidence {
	level := 0
	if indConf >= 2 || existsHumanApprove(obs) {
		level = 2
	} else if indConf == 1 {
		level = 1
	}
	if c := approveSetConfidence(obs); c != "" {
		level = confRank[c]
	}
	level -= countKind(obs, model.KindContradict)
	if level < 0 {
		level = 0
	}
	return rankConf[level]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/derive/... -run TestConfidence`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/confidence.go internal/derive/confidence_test.go
git commit -m "feat(derive): confidence computation"
```

---

### Task 7: Risk

**Files:**
- Create: `internal/derive/risk.go`
- Test: `internal/derive/risk_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/derive/risk_test.go`:
```go
package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func TestRiskForScope(t *testing.T) {
	p := policy.Default()

	cases := []struct {
		name  string
		mem   model.Memory
		scope model.Scope
		want  model.Risk
	}{
		{
			name:  "failed_attempt in migrations escalates to high",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"billing/migrations/**"}},
			want:  model.RiskHigh,
		},
		{
			name:  "stale_doc stays low",
			mem:   model.Memory{Type: model.TypeStaleDoc},
			scope: model.Scope{Paths: []string{"docs/setup.md"}},
			want:  model.RiskLow,
		},
		{
			name:  "external constraint is at least high",
			mem:   model.Memory{Type: model.TypeConstraint, Origin: model.OriginExternal},
			scope: model.Scope{Paths: []string{"api/handlers.go"}},
			want:  model.RiskHigh,
		},
		{
			name:  "auth path is critical",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"internal/auth/**"}},
			want:  model.RiskCritical,
		},
		{
			// "**" is broad (medium→high) AND, being the catch-all, intersects
			// every sensitive path — including auth (critical). Catch-all wins.
			name:  "catch-all scope is critical",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"**"}},
			want:  model.RiskCritical,
		},
	}
	for _, c := range cases {
		if got := riskForScope(c.mem, c.scope, p); got != c.want {
			t.Errorf("%s: risk = %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/... -run TestRiskForScope`
Expected: FAIL — `undefined: riskForScope`.

- [ ] **Step 3: Write `internal/derive/risk.go`**

```go
package derive

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

var riskRank = map[model.Risk]int{
	model.RiskLow: 0, model.RiskMedium: 1, model.RiskHigh: 2, model.RiskCritical: 3,
}
var rankRisk = []model.Risk{model.RiskLow, model.RiskMedium, model.RiskHigh, model.RiskCritical}

func maxRisk(a, b model.Risk) model.Risk {
	if riskRank[a] >= riskRank[b] {
		return a
	}
	return b
}

func bumpRisk(r model.Risk, n int) model.Risk {
	i := riskRank[r] + n
	if i > 3 {
		i = 3
	}
	if i < 0 {
		i = 0
	}
	return rankRisk[i]
}

// riskForScope implements prd.md §8.1: base risk by type, external-constraint
// floor, broad-scope bump, then sensitive-path escalation.
func riskForScope(m model.Memory, scope model.Scope, p policy.Policy) model.Risk {
	base, ok := p.BaseRisk[m.Type]
	if !ok {
		base = model.RiskMedium
	}
	if m.Type == model.TypeConstraint && m.Origin == model.OriginExternal {
		base = maxRisk(base, model.RiskHigh)
	}
	r := base
	if p.Escalators.BroadScopeBump && scopeIsBroad(scope) {
		r = bumpRisk(r, 1)
	}
	for _, sp := range p.Escalators.SensitivePaths {
		if scopeIntersectsGlob(scope, sp.Glob) {
			r = maxRisk(r, sp.MinRisk)
		}
	}
	return r
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/derive/... -run TestRiskForScope`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/risk.go internal/derive/risk_test.go
git commit -m "feat(derive): risk computation"
```

---

### Task 8: Enforcement

**Files:**
- Create: `internal/derive/enforcement.go`
- Test: `internal/derive/enforcement_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/derive/enforcement_test.go`:
```go
package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func TestEnforcement(t *testing.T) {
	p := policy.Default()

	cases := []struct {
		name   string
		obs    []model.Observation
		status model.Status
		risk   model.Risk
		want   model.Enforcement
	}{
		{"provisional → hint", nil, model.StatusProvisional, model.RiskHigh, model.EnforcementHint},
		{"active high → warning", nil, model.StatusActive, model.RiskHigh, model.EnforcementWarning},
		{"active low → recommendation", nil, model.StatusActive, model.RiskLow, model.EnforcementRecommendation},
		{
			"human sets requirement",
			[]model.Observation{{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, SetEnforcement: model.EnforcementRequirement}},
			model.StatusActive, model.RiskHigh, model.EnforcementRequirement,
		},
		{"contested → hint", nil, model.StatusContested, model.RiskHigh, model.EnforcementHint},
	}
	for _, c := range cases {
		if got := computeEnforcement(c.obs, c.status, c.risk, p); got != c.want {
			t.Errorf("%s: enforcement = %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/derive/... -run TestEnforcement`
Expected: FAIL — `undefined: computeEnforcement`.

- [ ] **Step 3: Write `internal/derive/enforcement.go`**

```go
package derive

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func approveSetEnforcement(obs []model.Observation) model.Enforcement {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.SetEnforcement != "" {
			return o.SetEnforcement
		}
	}
	return ""
}

// computeEnforcement implements prd.md §8.4: a human-set enforcement wins;
// otherwise active memories take their risk tier's max_auto_enforcement, and
// everything else surfaces as a hint. requirement is reachable only via approve.
func computeEnforcement(obs []model.Observation, status model.Status, risk model.Risk, p policy.Policy) model.Enforcement {
	if e := approveSetEnforcement(obs); e != "" {
		return e
	}
	if status == model.StatusActive {
		if max := p.Activation.Tiers[risk].MaxAutoEnforcement; max != "" {
			return max
		}
		return model.EnforcementRecommendation
	}
	return model.EnforcementHint
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/derive/... -run TestEnforcement`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/derive/enforcement.go internal/derive/enforcement_test.go
git commit -m "feat(derive): enforcement computation"
```

---

### Task 9: Derive orchestrator + golden lifecycle scenarios

**Files:**
- Create: `internal/derive/derive.go`
- Create: `internal/derive/derive_test.go`
- Create: `internal/derive/testdata/demo_provisional.yaml`, `demo_active.yaml`, `demo_approved.yaml`

- [ ] **Step 1: Write the golden fixtures**

Create `internal/derive/testdata/demo_provisional.yaml`:
```yaml
memory:
  id: m1
  type: failed_attempt
  title: Billing migrations require downgrade-path tests
  scope:
    paths: ["billing/migrations/**"]
  code_context:
    branch: feature/invoice-state
  actor:
    kind: agent
    name: claude-code
    session_id: session_123
  created_at: "2026-06-15T10:00:00Z"
observations: []
expected:
  status: provisional
  risk: high
  confidence: low
  enforcement: hint
  effective_scope: ["billing/migrations/**"]
```

Create `internal/derive/testdata/demo_active.yaml`:
```yaml
memory:
  id: m1
  type: failed_attempt
  title: Billing migrations require downgrade-path tests
  scope:
    paths: ["billing/migrations/**"]
  code_context:
    branch: feature/invoice-state
  actor:
    kind: agent
    name: claude-code
    session_id: session_123
  created_at: "2026-06-15T10:00:00Z"
observations:
  - id: o1
    target: m1
    kind: confirm
    code_context:
      branch: feature/revenue-reporting
    actor:
      kind: agent
      name: codex
      session_id: session_456
    created_at: "2026-06-15T11:20:00Z"
expected:
  status: active
  risk: high
  confidence: medium
  enforcement: warning
  effective_scope: ["billing/migrations/**"]
```

Create `internal/derive/testdata/demo_approved.yaml`:
```yaml
memory:
  id: m1
  type: failed_attempt
  title: Billing migrations require downgrade-path tests
  scope:
    paths: ["billing/migrations/**"]
  actor:
    kind: agent
    name: claude-code
    session_id: session_123
  created_at: "2026-06-15T10:00:00Z"
observations:
  - id: o1
    target: m1
    kind: confirm
    actor: { kind: agent, name: codex, session_id: session_456 }
    created_at: "2026-06-15T11:20:00Z"
  - id: o2
    target: m1
    kind: approve
    set_enforcement: requirement
    set_confidence: high
    actor: { kind: human, name: maintainer }
    created_at: "2026-06-15T12:00:00Z"
expected:
  status: active
  risk: high
  confidence: high
  enforcement: requirement
  effective_scope: ["billing/migrations/**"]
```

- [ ] **Step 2: Write the failing test**

Create `internal/derive/derive_test.go`:
```go
package derive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"gopkg.in/yaml.v3"
)

type goldenCase struct {
	Memory       model.Memory        `yaml:"memory"`
	Observations []model.Observation `yaml:"observations"`
	Expected     struct {
		Status         model.Status      `yaml:"status"`
		Risk           model.Risk        `yaml:"risk"`
		Confidence     model.Confidence  `yaml:"confidence"`
		Enforcement    model.Enforcement `yaml:"enforcement"`
		EffectiveScope []string          `yaml:"effective_scope"`
	} `yaml:"expected"`
}

func TestDeriveGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden fixtures found")
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var gc goldenCase
			if err := yaml.Unmarshal(data, &gc); err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			got := Derive(gc.Memory, gc.Observations, policy.Default())
			if got.Status != gc.Expected.Status {
				t.Errorf("status = %q, want %q", got.Status, gc.Expected.Status)
			}
			if got.Risk != gc.Expected.Risk {
				t.Errorf("risk = %q, want %q", got.Risk, gc.Expected.Risk)
			}
			if got.Confidence != gc.Expected.Confidence {
				t.Errorf("confidence = %q, want %q", got.Confidence, gc.Expected.Confidence)
			}
			if got.Enforcement != gc.Expected.Enforcement {
				t.Errorf("enforcement = %q, want %q", got.Enforcement, gc.Expected.Enforcement)
			}
			if len(got.EffectiveScope.Paths) != len(gc.Expected.EffectiveScope) {
				t.Fatalf("effective scope = %v, want %v", got.EffectiveScope.Paths, gc.Expected.EffectiveScope)
			}
			for i, p := range gc.Expected.EffectiveScope {
				if got.EffectiveScope.Paths[i] != p {
					t.Errorf("effective scope[%d] = %q, want %q", i, got.EffectiveScope.Paths[i], p)
				}
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/derive/... -run TestDeriveGolden`
Expected: FAIL — `undefined: Derive`.

- [ ] **Step 4: Write `internal/derive/derive.go`**

```go
// Package derive computes a memory's effective state — status, risk,
// confidence, enforcement, and effective scope — as a pure function of the
// memory, its observations, and policy (prd.md §8). It performs no I/O.
package derive

import (
	"fmt"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// DerivedState is the computed state of a memory at a point in time.
type DerivedState struct {
	Status         model.Status
	Risk           model.Risk
	Confidence     model.Confidence
	Enforcement    model.Enforcement
	EffectiveScope model.Scope

	IndependentConfirms int
	Contradictions      int
	Reason              string
}

// Derive computes the full state. Order matters: effective scope first (it can
// change which sensitive paths the scope touches), then risk on that scope,
// then status, confidence, and enforcement.
func Derive(m model.Memory, obs []model.Observation, p policy.Policy) DerivedState {
	eff := effectiveScope(m, obs)
	risk := riskForScope(m, eff, p)
	status, indConf := computeStatus(m, obs, risk, p)
	conf := computeConfidence(obs, indConf)
	enf := computeEnforcement(obs, status, risk, p)

	return DerivedState{
		Status:              status,
		Risk:                risk,
		Confidence:          conf,
		Enforcement:         enf,
		EffectiveScope:      eff,
		IndependentConfirms: indConf,
		Contradictions:      countKind(obs, model.KindContradict),
		Reason:              buildReason(status, indConf, obs),
	}
}

func buildReason(status model.Status, indConf int, obs []model.Observation) string {
	switch status {
	case model.StatusActive:
		if existsHumanApprove(obs) {
			return "approved by a maintainer"
		}
		return fmt.Sprintf("%d independent confirmation(s)", indConf)
	case model.StatusContested:
		return "an unresolved contradiction is on record"
	case model.StatusStale:
		return "marked stale and not since reconfirmed"
	case model.StatusRejected:
		return "rejected by a maintainer"
	default:
		return "awaiting independent confirmation"
	}
}
```

- [ ] **Step 5: Run the full package test suite**

Run: `go test ./...`
Expected: PASS across `model`, `policy`, and `derive`, including all three golden scenarios (which encode the flagship demo's provisional → active → approved progression).

- [ ] **Step 6: Run vet and verify the build**

Run: `go vet ./... && go build ./...`
Expected: no output, exit 0.

- [ ] **Step 7: Commit**

```bash
git add internal/derive/derive.go internal/derive/derive_test.go internal/derive/testdata/
git commit -m "feat(derive): Derive orchestrator with golden lifecycle scenarios"
```

---

## Definition of done

- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] `Derive` reproduces the flagship-demo lifecycle (the three golden fixtures) exactly: provisional/high/low/hint → active/high/medium/warning → active/high/high/requirement.
- [ ] Risk, status, confidence, enforcement, and effective scope each have dedicated tests covering the prd.md §8 rules, including: low-risk immediate activation, critical needs-human, contradiction→contested and resolution, narrowing-immediate vs broadening-substantiated, external-constraint floor, sensitive-path and broad-scope escalation.
- [ ] No package in this slice imports `os/exec`, `database/sql`, or any git/SQLite library — the engine stays pure.
