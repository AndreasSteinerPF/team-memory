package cli

import (
	"fmt"

	tmgit "github.com/AndreasSteinerPF/team-memory/internal/git"
)

// pushFailureHumanKind maps a PushFailureKind into a short human label used in
// status/doctor/sync output (spec §3.3).
func pushFailureHumanKind(k tmgit.PushFailureKind) string {
	switch k {
	case tmgit.KindProtectedBranch:
		return "protected branch"
	case tmgit.KindAuth:
		return "authentication failed"
	case tmgit.KindNetwork:
		return "network unreachable"
	default:
		return "unknown"
	}
}

// pushFailureFixHint returns a one-line, kind-specific suggestion for the user.
func pushFailureFixHint(rec *tmgit.PushFailureRecord) string {
	switch rec.Kind {
	case tmgit.KindProtectedBranch:
		return "tm remote set git@host:org/repo-memory.git    (see `tm remote --help`)"
	case tmgit.KindAuth:
		return fmt.Sprintf("check credentials for remote %q (ssh key / token), then `tm sync`", rec.Remote)
	case tmgit.KindNetwork:
		return "transient; verify connectivity and re-run `tm sync`"
	default:
		if rec.StderrExcerpt != "" {
			return "stderr: " + rec.StderrExcerpt
		}
		return "see `tm doctor` for details"
	}
}
