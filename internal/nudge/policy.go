package nudge

import "fmt"

// Nudge is the engine's output: one line of context to inject, plus metadata so
// the caller can record it in the journal for dedup/budget.
type Nudge struct {
	Text string
	Verb string // "propose" | "observe" | "" (self-review)
	Key  string // dedup key recorded into FiredNudge
}

// priority orders surviving signals: observe signals first (they unblock
// provisional→active and decay if neglected), then fail_pass > revert > churn.
var priority = map[SignalType]int{
	SigUnobserved: 0, SigDrift: 1, SigFailPass: 2, SigRevert: 3, SigChurn: 4,
}

// Decide applies the anti-spam policy and returns at most one nudge for the
// turn. acted(s) reports whether the agent already proposed/observed for signal
// s this session (injected so this package needs no ledger/index dependency).
func Decide(j *Journal, cfg Config, acted func(Signal) bool) (Nudge, bool) {
	if !cfg.Enabled {
		return Nudge{}, false
	}
	if len(j.Fired) >= cfg.MaxPerSession {
		return Nudge{}, false
	}
	if lastFiredTurn(j) >= 0 && j.Turn-lastFiredTurn(j) < cfg.CooldownTurns {
		return Nudge{}, false
	}

	sigs := Detect(j, cfg)

	// Tier A: self-classifying. Drop already-nudged (dedup) and already-acted.
	best, ok := bestTierA(j, sigs, acted)
	if ok {
		return renderTierA(best), true
	}

	// Tier B: attention-flag → aimed self-review.
	for _, s := range sigs {
		if s.Type == SigIntervened && !firedKey(j, s.Key()) {
			return Nudge{
				Text: fmt.Sprintf("tm: the user redirected you while editing %s — was there a constraint or decision worth recording? If so, tm_propose it; otherwise ignore.", s.Path),
				Verb: "", Key: s.Key(),
			}, true
		}
	}

	// Periodic generic self-review.
	if j.Turn >= cfg.SelfReviewEvery && len(j.Edits) > 0 {
		return Nudge{
			Text: "tm: anything memory-worthy this session — a non-obvious failure, a hidden constraint, a fragile area? If so, tm_propose it; otherwise ignore.",
			Verb: "", Key: fmt.Sprintf("self_review:%d", j.Turn),
		}, true
	}
	return Nudge{}, false
}

func bestTierA(j *Journal, sigs []Signal, acted func(Signal) bool) (Signal, bool) {
	var best Signal
	found := false
	for _, s := range sigs {
		if s.Type == SigIntervened {
			continue
		}
		if firedKey(j, s.Key()) || acted(s) {
			continue
		}
		if !found || priority[s.Type] < priority[best.Type] {
			best, found = s, true
		}
	}
	return best, found
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
	return Nudge{Text: text, Verb: s.Verb, Key: s.Key()}
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
