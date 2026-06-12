package derive

import (
	"strings"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// segments splits a glob into path segments, trimming slashes.
func segments(glob string) []string {
	glob = strings.Trim(glob, "/")
	if glob == "" {
		return nil
	}
	return strings.Split(glob, "/")
}

func hasWildcard(seg string) bool {
	return strings.ContainsAny(seg, "*?[")
}

// matchesEverything reports whether the glob is the catch-all "**".
func matchesEverything(glob string) bool {
	s := segments(glob)
	return len(s) == 1 && s[0] == "**"
}

// globIsBroad: the glob's first segment is itself a wildcard, so it can match
// paths across more than one top-level directory.
func globIsBroad(glob string) bool {
	s := segments(glob)
	if len(s) == 0 {
		return true
	}
	return hasWildcard(s[0])
}

func scopeIsBroad(s model.Scope) bool {
	for _, g := range s.Paths {
		if globIsBroad(g) {
			return true
		}
	}
	return false
}

// literalSegments returns the non-wildcard segments of a glob, in order.
func literalSegments(glob string) []string {
	var out []string
	for _, s := range segments(glob) {
		if !hasWildcard(s) {
			out = append(out, s)
		}
	}
	return out
}

// orderedSubsequence reports whether all of sub appear in seq in order.
func orderedSubsequence(sub, seq []string) bool {
	i := 0
	for _, s := range seq {
		if i < len(sub) && s == sub[i] {
			i++
		}
	}
	return i == len(sub)
}

// globIntersects: two globs intersect if either is the catch-all, or one's
// literal segments are an ordered subsequence of the other's. Simple and
// segment-exact (no partial-token matching). Full glob intersection is roadmap.
func globIntersects(a, b string) bool {
	if matchesEverything(a) || matchesEverything(b) {
		return true
	}
	la, lb := literalSegments(a), literalSegments(b)
	return orderedSubsequence(la, lb) || orderedSubsequence(lb, la)
}

func scopeIntersectsGlob(s model.Scope, glob string) bool {
	for _, g := range s.Paths {
		if globIntersects(g, glob) {
			return true
		}
	}
	return false
}

// literalPrefix returns the leading non-wildcard segments of a glob.
func literalPrefix(glob string) []string {
	var out []string
	for _, s := range segments(glob) {
		if hasWildcard(s) {
			break
		}
		out = append(out, s)
	}
	return out
}

// globContains: does outer contain inner (inner ⊆ outer)? True when outer is
// the catch-all, or outer's literal prefix is a segment-prefix of inner.
func globContains(outer, inner string) bool {
	if matchesEverything(outer) {
		return true
	}
	lp := literalPrefix(outer)
	ins := segments(inner)
	if len(lp) > len(ins) {
		return false
	}
	for i, seg := range lp {
		if ins[i] != seg {
			return false
		}
	}
	return true
}

// scopeSubset: every glob in inner is contained by some glob in outer.
func scopeSubset(inner, outer model.Scope) bool {
	for _, ig := range inner.Paths {
		ok := false
		for _, og := range outer.Paths {
			if globContains(og, ig) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// pathMatchesGlob matches a concrete path against a glob. "**" matches the rest;
// a single-segment wildcard matches exactly one segment.
func pathMatchesGlob(path, glob string) bool {
	if matchesEverything(glob) {
		return true
	}
	gsegs := segments(glob)
	psegs := segments(path)
	for i, g := range gsegs {
		if g == "**" {
			return true
		}
		if i >= len(psegs) {
			return false
		}
		if hasWildcard(g) {
			continue
		}
		if psegs[i] != g {
			return false
		}
	}
	return len(psegs) == len(gsegs)
}

func pathMatchesScope(path string, s model.Scope) bool {
	for _, g := range s.Paths {
		if pathMatchesGlob(path, g) {
			return true
		}
	}
	return false
}

// MatchPathGlob reports whether a concrete path matches a single glob, using
// TeamMemory's segment-exact glob semantics. Exported for the retrieval layer.
func MatchPathGlob(path, glob string) bool { return pathMatchesGlob(path, glob) }

// MatchPath reports whether a concrete path matches any glob in scope.
func MatchPath(path string, s model.Scope) bool { return pathMatchesScope(path, s) }

// effectiveScope applies adjust_scope observations in chronological order.
// Narrowings apply immediately; broadenings apply only once substantiated.
func effectiveScope(m model.Memory, obs []model.Observation) model.Scope {
	cur := m.Scope
	for _, a := range sortedByTime(filterKind(obs, model.KindAdjustScope)) {
		if a.SuggestedScope == nil {
			continue
		}
		sug := *a.SuggestedScope
		if scopeSubset(sug, cur) { // narrowing
			cur = sug
			continue
		}
		if broadeningSubstantiated(a, m, obs) {
			cur = sug
		}
	}
	return cur
}

// broadeningSubstantiated implements prd.md §8.5(a)/(b): a human approve after
// the adjustment, or a later independent confirm whose code-context paths fall
// inside the suggested scope but outside the original scope.
func broadeningSubstantiated(a model.Observation, m model.Memory, obs []model.Observation) bool {
	for _, o := range obs {
		if o.Kind == model.KindApprove && o.Actor.Kind == model.ActorHuman && o.CreatedAt.After(a.CreatedAt) {
			return true
		}
	}
	sug := *a.SuggestedScope
	prior := m.Scope
	for _, o := range obs {
		if o.Kind != model.KindConfirm || !o.CreatedAt.After(a.CreatedAt) {
			continue
		}
		if !isIndependent(o, m, "different_session") || o.CodeContext == nil {
			continue
		}
		matchSug, matchPrior := false, false
		for _, p := range o.CodeContext.Paths {
			if pathMatchesScope(p, sug) {
				matchSug = true
			}
			if pathMatchesScope(p, prior) {
				matchPrior = true
			}
		}
		if matchSug && !matchPrior {
			return true
		}
	}
	return false
}
