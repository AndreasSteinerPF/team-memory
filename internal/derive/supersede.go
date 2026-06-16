package derive

import (
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// Context carries cross-memory derived state. It is computed once over the
// whole ledger by BuildContext and passed into Derive so per-memory
// computation can see facts that depend on other memories' observations —
// today, only supersession (prd.md §5.3, §8.2).
type Context struct {
	// SupersededBy maps an obsolete memory ID to the new canonical that
	// supersedes it (substantiated per prd.md §8.5).
	SupersededBy map[string]string

	// pendingByTarget is the set of supersede observations that exist but
	// have not been substantiated, keyed by the *superseded* (obsolete) memory ID.
	pendingByTarget map[string][]model.Observation
}

// PendingSupersedeFor returns supersede observations naming b in `supersedes`
// that have not yet been substantiated. Used by `tm show` to surface pending
// claims.
func (c Context) PendingSupersedeFor(b string) []model.Observation {
	return c.pendingByTarget[b]
}

// BuildContext scans the full ledger and computes cross-memory state. Safe to
// call with empty memories/obs (returns a zero-but-non-nil Context).
func BuildContext(memories []model.Memory, allObs []model.Observation, p policy.Policy) Context {
	ctx := Context{
		SupersededBy:    make(map[string]string),
		pendingByTarget: make(map[string][]model.Observation),
	}

	// Fast path: nearly every ledger has zero supersede observations, so
	// avoid the O(N log N) sort over all observations in the hook hot path.
	var supersedes []model.Observation
	for _, o := range allObs {
		if o.Kind == model.KindSupersede {
			supersedes = append(supersedes, o)
		}
	}
	if len(supersedes) == 0 {
		return ctx
	}

	memByID := make(map[string]model.Memory, len(memories))
	for _, m := range memories {
		memByID[m.ID] = m
	}
	obsByTarget := make(map[string][]model.Observation, len(memByID))
	for _, o := range allObs {
		obsByTarget[o.Target] = append(obsByTarget[o.Target], o)
	}

	// Process supersede observations in chronological order so that, if
	// multiple supersede observations name the same B, the latest substantiated
	// one wins (matches §8.5's "latest applicable adjustment wins" intuition).
	for _, o := range sortedByTime(supersedes) {
		if o.Kind != model.KindSupersede {
			continue
		}
		if o.Supersedes == "" || o.Supersedes == o.Target {
			continue
		}
		a, ok := memByID[o.Target]
		if !ok {
			continue // supersede targets a memory we don't have
		}
		if _, exists := memByID[o.Supersedes]; !exists {
			continue // the obsolete memory isn't in the ledger
		}
		if supersedeSubstantiated(o, a, obsByTarget[a.ID], p) {
			ctx.SupersededBy[o.Supersedes] = a.ID
		} else {
			ctx.pendingByTarget[o.Supersedes] = append(ctx.pendingByTarget[o.Supersedes], o)
		}
	}

	return ctx
}

// supersedeSubstantiated mirrors broadeningSubstantiated (scope.go) but reads
// "later observations on A" rather than "later observations on the same memory
// as the adjust_scope." See prd.md §8.5.
func supersedeSubstantiated(o model.Observation, a model.Memory, aObs []model.Observation, p policy.Policy) bool {
	// (a) human approve on A after o
	for _, x := range aObs {
		if x.Kind == model.KindApprove && x.Actor.Kind == model.ActorHuman && x.CreatedAt.After(o.CreatedAt) {
			return true
		}
	}
	// (b) later independent confirm on A
	for _, x := range aObs {
		if x.Kind != model.KindConfirm || !x.CreatedAt.After(o.CreatedAt) {
			continue
		}
		if isIndependent(x, a, p.Activation.Independence) {
			return true
		}
	}
	return false
}
