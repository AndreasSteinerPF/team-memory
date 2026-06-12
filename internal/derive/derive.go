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
