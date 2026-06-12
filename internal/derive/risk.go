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
