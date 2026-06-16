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
		threshold := p.Activation.Tiers[risk].MinIndependentConfirms
		if threshold < 1 {
			threshold = 1 // omitted/0 ⇒ 1 (back-compat for medium/high)
		}
		return indConf >= threshold || existsHumanApprove(obs)
	default: // "never" or unknown
		return existsHumanApprove(obs)
	}
}

// computeStatus is the back-compat per-memory wrapper.
func computeStatus(m model.Memory, obs []model.Observation, risk model.Risk, p policy.Policy) (model.Status, int) {
	return computeStatusWithContext(m, obs, risk, p, Context{})
}

// computeStatusWithContext implements prd.md §8.2's precedence ladder using
// cross-memory state from ctx where needed (currently: SupersededBy).
func computeStatusWithContext(m model.Memory, obs []model.Observation, risk model.Risk, p policy.Policy, ctx Context) (model.Status, int) {
	indConf := countIndependentConfirms(m, obs, p.Activation.Independence)
	_, isSuperseded := ctx.SupersededBy[m.ID]
	switch {
	case existsKind(obs, model.KindReject):
		return model.StatusRejected, indConf
	case unresolved(obs, model.KindMarkStale):
		return model.StatusStale, indConf
	case unresolved(obs, model.KindMarkDuplicate):
		return model.StatusDuplicate, indConf
	case isSuperseded:
		return model.StatusSuperseded, indConf
	case unresolved(obs, model.KindContradict):
		return model.StatusContested, indConf
	case m.Type == model.TypeSuccessfulPattern && indConf == 0 && !existsHumanApprove(obs):
		// Type-specific activation gate (prd.md §8.2): successful_pattern stays
		// provisional until validated, regardless of its low-risk tier default.
		return model.StatusProvisional, indConf
	case isActive(obs, risk, indConf, p):
		return model.StatusActive, indConf
	default:
		return model.StatusProvisional, indConf
	}
}
