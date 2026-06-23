package nudge

import "fmt"

// SuppressionReason identifies why a candidate nudge did not fire.
type SuppressionReason string

const (
	SuppressDisabled      SuppressionReason = "disabled"
	SuppressMaxPerSession SuppressionReason = "max_per_session"
	SuppressCooldown      SuppressionReason = "cooldown"
	SuppressDedup         SuppressionReason = "dedup"
	SuppressAlreadyActed  SuppressionReason = "already_acted"
)

// Nudge is the engine's output: one line of context to inject, plus metadata so
// the caller can record it in the journal for dedup, budget, and reporting.
type Nudge struct {
	Text     string
	Verb     string // "propose" | "observe" | "" (self-review)
	Key      string // dedup key recorded into FiredNudge
	Type     SignalType
	Path     string
	MemoryID string
}

// Suppression records a candidate nudge suppressed by policy.
type Suppression struct {
	Reason   SuppressionReason `json:"reason"`
	Type     SignalType        `json:"type,omitempty"`
	Verb     string            `json:"verb,omitempty"`
	Path     string            `json:"path,omitempty"`
	MemoryID string            `json:"memory_id,omitempty"`
	Turn     int               `json:"turn"`
}

// Decision is the complete policy result for one Stop-hook turn.
type Decision struct {
	Nudge        Nudge
	Fired        bool
	Suppressions []Suppression
}

// priority orders surviving signals: observe signals first (they unblock
// provisional→active and decay if neglected), then fail_pass > revert > churn.
var priority = map[SignalType]int{
	SigUnobserved: 0, SigDrift: 1, SigFailPass: 2, SigRevert: 3, SigChurn: 4,
}

// Decide applies the anti-spam policy and returns at most one nudge for the turn
// plus any candidate suppressions. acted(s) reports whether the agent already
// proposed/observed for signal s this session (injected so this package needs no
// ledger/index dependency).
func Decide(j *Journal, cfg Config, acted func(Signal) bool) Decision {
	sigs := Detect(j, cfg)
	if !cfg.Enabled {
		return Decision{Suppressions: suppressAll(SuppressDisabled, j.Turn, sigs)}
	}
	if len(j.Fired) >= cfg.MaxPerSession {
		return Decision{Suppressions: suppressAll(SuppressMaxPerSession, j.Turn, sigs)}
	}
	if lastFiredTurn(j) >= 0 && j.Turn-lastFiredTurn(j) < cfg.CooldownTurns {
		return Decision{Suppressions: suppressAll(SuppressCooldown, j.Turn, sigs)}
	}

	// Tier A: self-classifying. Drop already-nudged (dedup) and already-acted.
	best, suppressed := bestTierA(j, sigs, acted)
	if best.Type != "" {
		return Decision{Nudge: renderTierA(best), Fired: true, Suppressions: suppressed}
	}
	if len(suppressed) > 0 {
		return Decision{Suppressions: suppressed}
	}

	// Tier B: attention-flag → aimed self-review.
	for _, s := range sigs {
		if s.Type == SigIntervened && !firedKey(j, s.Key()) {
			return Decision{Nudge: Nudge{
				Text: fmt.Sprintf("tm: the user redirected you while editing %s — was there a constraint or decision worth recording? If so, tm_propose it; otherwise ignore.", s.Path),
				Verb: "", Key: s.Key(), Type: s.Type, Path: s.Path,
			}, Fired: true}
		}
		if s.Type == SigIntervened {
			suppressed = append(suppressed, suppression(SuppressDedup, j.Turn, s))
		}
	}
	if len(suppressed) > 0 {
		return Decision{Suppressions: suppressed}
	}

	// Periodic generic self-review.
	if j.Turn >= cfg.SelfReviewEvery && len(j.Edits) > 0 {
		return Decision{Nudge: Nudge{
			Text: "tm: anything memory-worthy this session — a non-obvious failure, a hidden constraint, a fragile area? If so, tm_propose it; otherwise ignore.",
			Verb: "", Key: fmt.Sprintf("self_review:%d", j.Turn), Type: SignalType("self_review"),
		}, Fired: true}
	}
	return Decision{}
}

func suppressAll(reason SuppressionReason, turn int, sigs []Signal) []Suppression {
	out := make([]Suppression, 0, len(sigs))
	for _, s := range sigs {
		out = append(out, suppression(reason, turn, s))
	}
	return out
}

func bestTierA(j *Journal, sigs []Signal, acted func(Signal) bool) (Signal, []Suppression) {
	var best Signal
	var suppressed []Suppression
	for _, s := range sigs {
		if s.Type == SigIntervened {
			continue
		}
		if firedKey(j, s.Key()) {
			suppressed = append(suppressed, suppression(SuppressDedup, j.Turn, s))
			continue
		}
		if acted(s) {
			suppressed = append(suppressed, suppression(SuppressAlreadyActed, j.Turn, s))
			continue
		}
		if best.Type == "" || priority[s.Type] < priority[best.Type] {
			best = s
		}
	}
	return best, suppressed
}

func renderTierA(s Signal) Nudge {
	var text string
	switch s.Type {
	case SigFailPass:
		text = fmt.Sprintf("tm: recovered from a failing `%s` after edits in %s — if that fix encodes a non-obvious lesson, tm_propose a failed_attempt; otherwise ignore.", s.Command, s.Path)
	case SigRevert:
		text = "tm: you reverted work this session — if an approach failed in a non-obvious way, tm_propose a failed_attempt; otherwise ignore."
	case SigChurn:
		text = fmt.Sprintf("tm: %s fought back (edited repeatedly) — if it's a fragile area, tm_propose a fragile_area; otherwise ignore.", s.Path)
	case SigUnobserved:
		text = fmt.Sprintf("tm: you were shown memory %s for %s and your work bears on it — tm_observe to confirm or contradict it (with evidence); otherwise ignore.", s.Memory, s.Path)
	case SigDrift:
		text = fmt.Sprintf("tm: memory %s is anchored to %s, which has drifted, and you just edited it — tm_observe mark_stale or adjust_scope; otherwise ignore.", s.Memory, s.Path)
	}
	return Nudge{Text: text, Verb: s.Verb, Key: s.Key(), Type: s.Type, Path: s.Path, MemoryID: s.Memory}
}

func lastFiredTurn(j *Journal) int {
	last := -1
	for _, f := range j.Fired {
		if f.Turn > last {
			last = f.Turn
		}
	}
	return last
}

func firedKey(j *Journal, key string) bool {
	for _, f := range j.Fired {
		if f.Key == key {
			return true
		}
	}
	return false
}
