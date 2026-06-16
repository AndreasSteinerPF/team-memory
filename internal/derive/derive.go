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

	// PendingAdjustments holds adjust_scope observations whose suggested scope
	// broadens the effective scope but is not yet substantiated (prd.md §8.5).
	// Visible in `tm show` so humans know what broadening requests are queued.
	PendingAdjustments []model.Observation
}

// Derive computes the full state for memory m given its own observations.
// Cross-memory state (currently: supersession) is computed as if there were
// no other memories — use DeriveWithContext for ledger-wide derivation.
func Derive(m model.Memory, obs []model.Observation, p policy.Policy) DerivedState {
	return DeriveWithContext(m, obs, p, Context{})
}

// DeriveWithContext computes derived state including cross-memory facts
// carried by ctx. Use it when you have already built a Context with
// BuildContext (typically the index replay or any CLI surface that has the
// full ledger in hand).
func DeriveWithContext(m model.Memory, obs []model.Observation, p policy.Policy, ctx Context) DerivedState {
	eff := effectiveScope(m, obs)
	risk := riskForScope(m, eff, p)
	status, indConf := computeStatusWithContext(m, obs, risk, p, ctx)
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
		Reason:              buildReasonWithContext(status, indConf, obs, m.ID, ctx),
		PendingAdjustments:  pendingBroadenings(m, obs),
	}
}

func buildReasonWithContext(status model.Status, indConf int, obs []model.Observation, memID string, ctx Context) string {
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
	case model.StatusDuplicate:
		if id := latestCanonicalID(obs); id != "" {
			return "duplicate of " + id
		}
		return "marked as a duplicate"
	case model.StatusSuperseded:
		if memID != "" {
			if a, ok := ctx.SupersededBy[memID]; ok {
				return "superseded by " + a
			}
		}
		return "superseded by a newer memory"
	case model.StatusRejected:
		return "rejected by a maintainer"
	default:
		return "awaiting independent confirmation"
	}
}

// latestCanonicalID returns the canonical_id from the most recent
// mark_duplicate observation, or "" if none.
func latestCanonicalID(obs []model.Observation) string {
	var latest model.Observation
	found := false
	for _, o := range obs {
		if o.Kind != model.KindMarkDuplicate {
			continue
		}
		if !found || o.CreatedAt.After(latest.CreatedAt) {
			latest = o
			found = true
		}
	}
	return latest.CanonicalID
}
