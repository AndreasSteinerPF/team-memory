package policy

import (
	"strings"
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
	if p.Activation.Tiers[model.RiskCritical].Auto != "independent_confirm" {
		t.Errorf("critical tier auto = %q, want independent_confirm", p.Activation.Tiers[model.RiskCritical].Auto)
	}
	if p.Activation.Tiers[model.RiskHigh].MaxAutoEnforcement != model.EnforcementWarning {
		t.Errorf("high tier max enforcement = %q, want warning", p.Activation.Tiers[model.RiskHigh].MaxAutoEnforcement)
	}
	if len(p.Escalators.SensitivePaths) == 0 {
		t.Error("expected default sensitive paths")
	}
}

func TestDefaultCriticalTierAutoActivates(t *testing.T) {
	tier := Default().Activation.Tiers[model.RiskCritical]
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

func TestDefaultNudgeConfig(t *testing.T) {
	p := Default()
	if !p.Nudge.Enabled {
		t.Errorf("Nudge.Enabled default = false, want true")
	}
	if p.Nudge.MaxPerSession != 3 {
		t.Errorf("Nudge.MaxPerSession = %d, want 3", p.Nudge.MaxPerSession)
	}
	if p.Nudge.CooldownTurns != 3 {
		t.Errorf("Nudge.CooldownTurns = %d, want 3", p.Nudge.CooldownTurns)
	}
	if p.Nudge.SelfReviewEvery != 8 {
		t.Errorf("Nudge.SelfReviewEvery = %d, want 8", p.Nudge.SelfReviewEvery)
	}
	if p.Nudge.ChurnThreshold != 3 {
		t.Errorf("Nudge.ChurnThreshold = %d, want 3", p.Nudge.ChurnThreshold)
	}
}

func TestNudgeConfigRoundTripsThroughYAML(t *testing.T) {
	data, err := DefaultYAML()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nudge != Default().Nudge {
		t.Errorf("Nudge round-trip = %+v, want %+v", p.Nudge, Default().Nudge)
	}
}

func TestRequirementEnforcementDefaultAndRoundTrip(t *testing.T) {
	p := Default()
	if !p.RequirementEnforcement.HumanRequired {
		t.Fatal("default requirement_enforcement.human_required must be true (prd.md §8.1)")
	}
	y, err := DefaultYAML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(y), "requirement_enforcement:") {
		t.Fatal("DefaultYAML must serialize the requirement_enforcement key")
	}
	p2, err := Load(y)
	if err != nil {
		t.Fatal(err)
	}
	if !p2.RequirementEnforcement.HumanRequired {
		t.Fatal("Load(DefaultYAML()) must preserve human_required=true")
	}
}

func TestDefaultInjectConfig(t *testing.T) {
	if Default().Inject.AdvisoryMaxPerSession != 5 {
		t.Errorf("Inject.AdvisoryMaxPerSession = %d, want 5", Default().Inject.AdvisoryMaxPerSession)
	}
}

func TestDefaultPolicyHasSuccessfulPatternRisk(t *testing.T) {
	p := Default()
	got, ok := p.BaseRisk[model.TypeSuccessfulPattern]
	if !ok {
		t.Fatal("BaseRisk missing successful_pattern")
	}
	if got != model.RiskLow {
		t.Fatalf("successful_pattern base risk: got %q, want %q", got, model.RiskLow)
	}
}
