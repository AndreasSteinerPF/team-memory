package nudge

// SignalType identifies a memory-worthy moment.
type SignalType string

const (
	SigFailPass   SignalType = "fail_pass"  // Tier A
	SigRevert     SignalType = "revert"     // Tier A
	SigChurn      SignalType = "churn"      // Tier A
	SigUnobserved SignalType = "unobserved" // Tier A (observe)
	SigDrift      SignalType = "drift"      // Tier A (observe)
	SigIntervened SignalType = "intervened" // Tier B (attention-flag)
)

// Config is the subset of policy.Nudge the detector and decider need. The CLI
// maps policy.Nudge → Config so the nudge package stays free of policy imports.
type Config struct {
	Enabled         bool
	MaxPerSession   int
	CooldownTurns   int
	SelfReviewEvery int
	ChurnThreshold  int
}

// Signal is one detected moment. Verb/MemType drive the nudge wording; Path and
// Memory key dedup and the suppress-if-acted check.
type Signal struct {
	Type    SignalType
	Verb    string // "propose" | "observe"
	MemType string // suggested memory type for propose signals
	Path    string // primary path (for dedup + acted check)
	Memory  string // memory id (for observe signals)
	Command string // command signature (for fail_pass wording)
}

// Key is the dedup key for a signal: one nudge per (type, path-or-memory).
func (s Signal) Key() string {
	id := s.Path
	if s.Memory != "" {
		id = s.Memory
	}
	return string(s.Type) + ":" + id
}

func suppression(reason SuppressionReason, turn int, s Signal) Suppression {
	return Suppression{
		Reason:   reason,
		Type:     s.Type,
		Verb:     s.Verb,
		Path:     s.Path,
		MemoryID: s.Memory,
		Turn:     turn,
	}
}

// Detect returns all signals currently present in the journal. Pure: no I/O,
// no ledger access. Suppress-if-acted and budget are applied later in Decide.
func Detect(j *Journal, cfg Config) []Signal {
	var out []Signal
	out = append(out, detectFailPass(j)...)
	out = append(out, detectRevert(j)...)
	out = append(out, detectChurn(j, cfg)...)
	out = append(out, detectObserve(j)...)
	out = append(out, detectIntervened(j)...)
	return out
}

// detectFailPass: a command signature that failed, then succeeded, with at
// least one edit between the two runs. Only the boolean transition matters.
func detectFailPass(j *Journal) []Signal {
	var out []Signal
	for i, fail := range j.Commands {
		if !fail.Failed {
			continue
		}
		for _, pass := range j.Commands[i+1:] {
			if pass.Failed || pass.Signature != fail.Signature {
				continue
			}
			if !editBetween(j, fail.Turn, pass.Turn) {
				continue
			}
			out = append(out, Signal{
				Type: SigFailPass, Verb: "propose", MemType: "failed_attempt",
				Path: lastEditBetween(j, fail.Turn, pass.Turn), Command: fail.Signature,
			})
			break
		}
	}
	return out
}

func editBetween(j *Journal, lo, hi int) bool {
	for _, e := range j.Edits {
		if e.Turn >= lo && e.Turn <= hi {
			return true
		}
	}
	return false
}

func lastEditBetween(j *Journal, lo, hi int) string {
	path := ""
	for _, e := range j.Edits {
		if e.Turn >= lo && e.Turn <= hi {
			path = e.Path
		}
	}
	return path
}

func detectRevert(j *Journal) []Signal {
	var out []Signal
	for range j.Reverts {
		out = append(out, Signal{Type: SigRevert, Verb: "propose", MemType: "failed_attempt"})
		break // one revert signal per session is enough; dedup handles the rest
	}
	return out
}

func detectChurn(j *Journal, cfg Config) []Signal {
	counts := map[string]int{}
	for _, e := range j.Edits {
		counts[e.Path]++
	}
	var out []Signal
	for path, n := range counts {
		if n >= cfg.ChurnThreshold {
			out = append(out, Signal{Type: SigChurn, Verb: "propose", MemType: "fragile_area", Path: path})
		}
	}
	return out
}

// detectObserve emits unobserved/drift signals for surfaced memories whose path
// the session subsequently edited.
func detectObserve(j *Journal) []Signal {
	var out []Signal
	for _, s := range j.Surfaced {
		if j.EditCount(s.Path) == 0 {
			continue
		}
		if s.Drift {
			out = append(out, Signal{Type: SigDrift, Verb: "observe", Path: s.Path, Memory: s.MemoryID})
		} else {
			out = append(out, Signal{Type: SigUnobserved, Verb: "observe", Path: s.Path, Memory: s.MemoryID})
		}
	}
	return out
}

// detectIntervened: edit P → user prompt → edit P again (Tier B). The lesson
// content lives in the user's words, so this only aims the self-review.
func detectIntervened(j *Journal) []Signal {
	var out []Signal
	for path := range pathsEditedAround(j) {
		out = append(out, Signal{Type: SigIntervened, Verb: "", Path: path})
	}
	return out
}

func pathsEditedAround(j *Journal) map[string]struct{} {
	found := map[string]struct{}{}
	for _, p := range j.PromptTurns {
		before := map[string]struct{}{}
		for _, e := range j.Edits {
			if e.Turn < p {
				before[e.Path] = struct{}{}
			}
		}
		for _, e := range j.Edits {
			if e.Turn > p {
				if _, ok := before[e.Path]; ok {
					found[e.Path] = struct{}{}
				}
			}
		}
	}
	return found
}
