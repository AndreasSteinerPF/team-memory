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
