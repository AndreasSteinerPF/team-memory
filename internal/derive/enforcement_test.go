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
