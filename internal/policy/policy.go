// Package policy defines the configurable policy that drives state derivation,
// its built-in defaults (matching prd.md §8.1), and YAML loading that merges
// user overrides onto those defaults.
package policy

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"gopkg.in/yaml.v3"
)

type Policy struct {
	BaseRisk               map[model.MemoryType]model.Risk `yaml:"base_risk"`
	Escalators             Escalators                      `yaml:"escalators"`
	Activation             Activation                      `yaml:"activation"`
	RequirementEnforcement RequirementEnforcement          `yaml:"requirement_enforcement"`
	Retrieval              Retrieval                       `yaml:"retrieval"`
	Sync                   Sync                            `yaml:"sync"`
	Nudge                  Nudge                           `yaml:"nudge"`
	Inject                 Inject                          `yaml:"inject"`
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
	Independence string              `yaml:"independence"` // different_session | different_session_and_branch
	Tiers        map[model.Risk]Tier `yaml:"tiers"`
}

type Tier struct {
	Auto                   string            `yaml:"auto"` // immediate | independent_confirm | never
	MinIndependentConfirms int               `yaml:"min_independent_confirms,omitempty"`
	MaxAutoEnforcement     model.Enforcement `yaml:"max_auto_enforcement,omitempty"`
}

// RequirementEnforcement mirrors prd.md §8.1. v1 derivation hard-codes
// requirement-via-human-approve regardless of this value (see
// internal/derive/enforcement.go); the key is parsed so policy.yaml matches
// the PRD's documented default exactly.
type RequirementEnforcement struct {
	HumanRequired bool `yaml:"human_required"`
}

type Retrieval struct {
	MaxResults      int    `yaml:"max_results"`
	MaxProvisional  int    `yaml:"max_provisional"`
	ProvisionalMode string `yaml:"provisional_mode"` // never | related | always
}

type Sync struct {
	AutoFetchAfter string `yaml:"auto_fetch_after"` // duration string, e.g. "5m"
}

// Inject configures post-tool advisory memory injection (prd.md §8.1 config, §10.6 cross-harness).
type Inject struct {
	AdvisoryMaxPerSession int `yaml:"advisory_max_per_session"`
}

// Nudge configures the near-moment proposing/observing nudge engine (prd.md §8.1 config, §10.1 hooks).
type Nudge struct {
	Enabled         bool `yaml:"enabled"`
	MaxPerSession   int  `yaml:"max_per_session"`
	CooldownTurns   int  `yaml:"cooldown_turns"`
	SelfReviewEvery int  `yaml:"self_review_every"`
	ChurnThreshold  int  `yaml:"churn_threshold"`
}

// Default returns the built-in policy from prd.md §8.1.
func Default() Policy {
	return Policy{
		BaseRisk: map[model.MemoryType]model.Risk{
			model.TypeStaleDoc:          model.RiskLow,
			model.TypeDecision:          model.RiskLow,
			model.TypeSuccessfulPattern: model.RiskLow, // prd.md §8.1
			model.TypeFailedAttempt:     model.RiskMedium,
			model.TypeFragileArea:       model.RiskMedium,
			model.TypeConstraint:        model.RiskMedium, // origin=external escalates to high in derive
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
				model.RiskCritical: {Auto: "independent_confirm", MinIndependentConfirms: 2, MaxAutoEnforcement: model.EnforcementWarning},
			},
		},
		RequirementEnforcement: RequirementEnforcement{HumanRequired: true},
		Retrieval:              Retrieval{MaxResults: 5, MaxProvisional: 2, ProvisionalMode: "related"},
		Sync:                   Sync{AutoFetchAfter: "5m"},
		Nudge: Nudge{
			Enabled:         true,
			MaxPerSession:   3,
			CooldownTurns:   3,
			SelfReviewEvery: 8,
			ChurnThreshold:  3,
		},
		Inject: Inject{AdvisoryMaxPerSession: 5},
	}
}

// Load parses YAML over a copy of Default(). yaml.v3 merges decoded keys into
// the existing (non-nil) maps rather than replacing them, so keys a user does
// not specify keep their default values.
func Load(data []byte) (Policy, error) {
	p := Default()
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Policy{}, err
	}
	return p, nil
}

// DefaultYAML returns the built-in policy serialized as YAML with a header
// comment, suitable for writing to a freshly initialized ledger's policy.yaml.
// Load(DefaultYAML()) equals Default().
func DefaultYAML() ([]byte, error) {
	body, err := yaml.Marshal(Default())
	if err != nil {
		return nil, err
	}
	header := "# TeamMemory policy (prd.md §8.1). Edit to tune risk, activation, retrieval, and sync.\n"
	return append([]byte(header), body...), nil
}
