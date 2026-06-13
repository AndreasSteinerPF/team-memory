package cli

import "errors"

type severity int

const (
	sevOK severity = iota
	sevWarn
	sevSkip
	sevFail
)

func (s severity) icon() string {
	switch s {
	case sevOK:
		return "✓"
	case sevWarn:
		return "⚠"
	case sevFail:
		return "✗"
	default: // sevSkip
		return "–"
	}
}

// checkResult is one diagnostic line. hint (a remediation command) is shown
// indented beneath the line when present.
type checkResult struct {
	name   string
	sev    severity
	detail string
	hint   string
}

// anyFailed reports whether any result is a hard failure — this drives the
// process exit code (exit 1 iff true).
func anyFailed(results []checkResult) bool {
	for _, r := range results {
		if r.sev == sevFail {
			return true
		}
	}
	return false
}

var errDoctorFailed = errors.New("one or more checks failed")
