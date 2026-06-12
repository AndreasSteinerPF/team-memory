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
