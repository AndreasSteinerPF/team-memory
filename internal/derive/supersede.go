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

// HasCycleBackTo reports whether the existing canonical/supersedes graph
// already contains a path that, combined with the new observation about to
// be filed, would form a cycle. Used by the CLI and MCP observe surfaces to
// warn (but not block) on cycles of any length (prd.md §8.5).
//
// Callers pass (a, b) = (target, cross-memory-ref) of the *new* observation:
//   - mark_duplicate: a is the duplicate, b is the canonical_id.
//     The new arc points a → b (a duplicates b). Cycle iff a path
//     b → ... → a already exists. We walk from b looking for a, following
//     "X is a duplicate of Y" arcs (obs target → canonical_id).
//   - supersede: a is the new canonical, b is the supersedes (obsolete).
//     The new arc points b → a (b is superseded by a). Cycle iff a path
//     a → ... → b already exists. We walk from a looking for b, following
//     "X is superseded by Y" arcs (obs supersedes → target).
//
// Resolved/unresolved state of intermediate observations is intentionally
// ignored — a cycle is still worth surfacing even if some legs were later
// confirmed.
func HasCycleBackTo(obs []model.Observation, a, b string, kind model.ObservationKind) bool {
	var start, target string
	switch kind {
	case model.KindMarkDuplicate:
		start, target = b, a
	case model.KindSupersede:
		start, target = a, b
	default:
		return false
	}

	successors := func(node string) []string {
		var next []string
		for _, o := range obs {
			if o.Kind != kind {
				continue
			}
			switch kind {
			case model.KindMarkDuplicate:
				if o.Target == node && o.CanonicalID != "" {
					next = append(next, o.CanonicalID)
				}
			case model.KindSupersede:
				if o.Supersedes == node && o.Target != "" {
					next = append(next, o.Target)
				}
			}
		}
		return next
	}

	visited := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, n := range successors(node) {
			if n == target {
				return true
			}
			if !visited[n] {
				visited[n] = true
				queue = append(queue, n)
			}
		}
	}
	return false
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
