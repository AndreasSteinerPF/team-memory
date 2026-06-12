// Package retrieve implements TeamMemory's precision-first, lexical retrieval
// (prd.md §11). Given an action (paths + description) it selects candidate
// memories from the index (scope-glob match or FTS match), filters by status,
// annotates anchor drift (prd.md §8.6), ranks them, and applies output caps.
package retrieve

import (
	"fmt"
	"sort"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// Provisional framing (prd.md §5.5, §11.3).
const (
	CautionFraming     = "Possible lesson from prior work. Use as caution, not policy. Add a confirmation or contradiction if your work bears on it."
	RequestObservation = "If your work bears on this, record a `confirm` or `contradict` observation with evidence."
)

// MatchKind records why a memory entered the candidate set.
type MatchKind string

const (
	MatchScope MatchKind = "scope" // an effective-scope glob matched an action path
	MatchFTS   MatchKind = "fts"   // the description matched title/summary/guidance
)

// Query describes the action being checked.
type Query struct {
	Paths           []string // target paths of the action (e.g. the file being edited)
	Description     string   // free-text action/plan description, searched via FTS
	ProvisionalMode string   // "" uses policy; one of "never" | "related" | "always"
}

// DriftInfo annotates an anchored file that has moved on since the memory was
// recorded (prd.md §8.6).
type DriftInfo struct {
	Path           string
	Exists         bool
	CommitsChanged int    // commits touching Path since the anchor commit; -1 if unknown
	Note           string // human-facing annotation
}

// Result is one retrieved memory with its match metadata and annotations.
type Result struct {
	Memory      index.IndexedMemory
	Match       MatchKind
	Specificity int         // glob specificity of the best scope match; 0 for FTS-only
	Provisional bool        // surfaced as caution rather than trusted guidance
	Caution     string      // provisional framing; empty for active
	Request     string      // requested-observation prompt; empty for active
	Drift       []DriftInfo // anchor-drift annotations; nil if nothing drifted
}

// Index is the read surface retrieval needs (satisfied by *index.Index).
type Index interface {
	All() ([]index.IndexedMemory, error)
	SearchIDs(query string) ([]string, error)
}

// DriftSource reports anchor drift for a path relative to an anchor commit,
// against the current code repository (prd.md §8.6). Implemented by GitDrift.
type DriftSource interface {
	// Drift reports whether path still exists at HEAD and how many commits have
	// touched it since sinceCommit. commitsChanged == -1 means the count could
	// not be determined (e.g. an unknown anchor commit).
	Drift(path, sinceCommit string) (exists bool, commitsChanged int, err error)
}

// Engine answers retrieval queries against an index.
type Engine struct {
	idx   Index
	drift DriftSource // may be nil to disable anchor-drift annotation
	pol   policy.Policy
}

// New builds an Engine. drift may be nil (no anchor-drift annotation).
func New(idx Index, drift DriftSource, pol policy.Policy) *Engine {
	return &Engine{idx: idx, drift: drift, pol: pol}
}

type candidate struct {
	mem         index.IndexedMemory
	match       MatchKind
	specificity int
	ftsRank     int // position in FTS results; -1 if not an FTS match
	provisional bool
	drift       []DriftInfo
	driftScore  int // higher ⇒ more drifted ⇒ ranks lower
}

// Retrieve returns the ranked, capped set of memories relevant to q.
func (e *Engine) Retrieve(q Query) ([]Result, error) {
	all, err := e.idx.All()
	if err != nil {
		return nil, err
	}

	ftsRank := map[string]int{}
	if fq := ftsQuery(q.Description); fq != "" {
		hits, err := e.idx.SearchIDs(fq)
		if err != nil {
			return nil, err
		}
		for i, id := range hits {
			if _, dup := ftsRank[id]; !dup {
				ftsRank[id] = i
			}
		}
	}

	mode := q.ProvisionalMode
	if mode == "" {
		mode = e.pol.Retrieval.ProvisionalMode
	}

	var active, prov []candidate
	for _, m := range all {
		if m.Status == model.StatusStale || m.Status == model.StatusRejected {
			continue // excluded from retrieval (prd.md §8.2)
		}
		spec, scopeMatch := bestSpecificity(m.EffectiveScope, q.Paths)
		fr, isFTS := ftsRank[m.ID]
		if !scopeMatch && !isFTS {
			continue
		}
		c := candidate{mem: m, specificity: spec, ftsRank: -1}
		if scopeMatch {
			c.match = MatchScope
		} else {
			c.match = MatchFTS
		}
		if isFTS {
			c.ftsRank = fr
		}
		switch m.Status {
		case model.StatusActive:
			active = append(active, c)
		case model.StatusProvisional, model.StatusContested:
			if mode == "never" {
				continue
			}
			if mode == "related" && !scopeMatch {
				continue // provisional appears only on scope match, not FTS-only
			}
			c.provisional = true
			prov = append(prov, c)
		}
	}

	e.annotateDrift(active)
	e.annotateDrift(prov)

	sortCandidates(active)
	sortCandidates(prov)

	return e.cap(active, prov), nil
}

// annotateDrift fills each candidate's drift annotations and drift score.
func (e *Engine) annotateDrift(cs []candidate) {
	if e.drift == nil {
		return
	}
	for i := range cs {
		for _, a := range cs[i].mem.Anchors {
			exists, changed, err := e.drift.Drift(a.Path, a.Commit)
			if err != nil {
				continue // drift lookup failure must not fail retrieval
			}
			switch {
			case !exists:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: false, CommitsChanged: 0, Note: noteMissing(a.Path)})
				cs[i].driftScore += 1000
			case changed < 0:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: true, CommitsChanged: -1, Note: noteUnknown(a.Path)})
				cs[i].driftScore += 50
			case changed > 0:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: true, CommitsChanged: changed, Note: noteChanged(a.Path, changed)})
				cs[i].driftScore += changed
			default:
				// fresh (changed == 0): no annotation, no penalty
			}
		}
	}
}

// cap fills active results first, then provisional up to max_provisional, all
// within max_results (prd.md §11.3, §11.4).
func (e *Engine) cap(active, prov []candidate) []Result {
	maxResults := e.pol.Retrieval.MaxResults
	maxProv := e.pol.Retrieval.MaxProvisional

	var out []Result
	for _, c := range active {
		if len(out) >= maxResults {
			break
		}
		out = append(out, toResult(c))
	}
	slots := maxProv
	if rem := maxResults - len(out); rem < slots {
		slots = rem
	}
	for i := 0; i < len(prov) && slots > 0; i++ {
		out = append(out, toResult(prov[i]))
		slots--
	}
	return out
}

func toResult(c candidate) Result {
	r := Result{
		Memory:      c.mem,
		Match:       c.match,
		Specificity: c.specificity,
		Provisional: c.provisional,
		Drift:       c.drift,
	}
	if c.provisional {
		r.Caution = CautionFraming
		r.Request = RequestObservation
	}
	return r
}

// sortCandidates orders by the prd.md §11 ranking: glob specificity > status
// (active first) > enforcement > confidence > recency > anchor freshness, with
// FTS rank and ID as deterministic final tiebreakers.
func sortCandidates(cs []candidate) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.specificity != b.specificity {
			return a.specificity > b.specificity
		}
		if sa, sb := statusRank(a.mem.Status), statusRank(b.mem.Status); sa != sb {
			return sa > sb
		}
		if ea, eb := enfRank(a.mem.Enforcement), enfRank(b.mem.Enforcement); ea != eb {
			return ea > eb
		}
		if ca, cb := confRank(a.mem.Confidence), confRank(b.mem.Confidence); ca != cb {
			return ca > cb
		}
		if !a.mem.CreatedAt.Equal(b.mem.CreatedAt) {
			return a.mem.CreatedAt.After(b.mem.CreatedAt)
		}
		if a.driftScore != b.driftScore {
			return a.driftScore < b.driftScore // fresher first
		}
		if ka, kb := ftsKey(a.ftsRank), ftsKey(b.ftsRank); ka != kb {
			return ka < kb
		}
		return a.mem.ID < b.mem.ID
	})
}

func ftsKey(r int) int {
	if r < 0 {
		return 1 << 30 // non-FTS matches sort after real FTS ranks among equals
	}
	return r
}

func statusRank(s model.Status) int {
	switch s {
	case model.StatusActive:
		return 2
	case model.StatusContested, model.StatusProvisional:
		return 1
	default:
		return 0
	}
}

func enfRank(e model.Enforcement) int {
	switch e {
	case model.EnforcementRequirement:
		return 3
	case model.EnforcementWarning:
		return 2
	case model.EnforcementRecommendation:
		return 1
	default:
		return 0
	}
}

func confRank(c model.Confidence) int {
	switch c {
	case model.ConfidenceHigh:
		return 2
	case model.ConfidenceMedium:
		return 1
	default:
		return 0
	}
}

func noteChanged(path string, n int) string {
	return fmt.Sprintf("anchored file %s has changed %d commit(s) since this memory was recorded — verify it still applies, and `mark_stale` if not.", path, n)
}

func noteMissing(path string) string {
	return fmt.Sprintf("anchored file %s no longer exists — verify this memory still applies, and `mark_stale` if not.", path)
}

func noteUnknown(path string) string {
	return fmt.Sprintf("anchored commit for %s was not found in history — verify this memory still applies.", path)
}
