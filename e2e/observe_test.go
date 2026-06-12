package e2e

import (
	"strings"
	"testing"
)

func TestObserveConfirmActivates(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	// Propose a medium-risk failed_attempt (session s1) → provisional.
	out, _, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "rollback needs downgrade tests",
		"--scope", "billing/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: provisional") {
		t.Fatalf("want provisional, got: %s", out)
	}
	id := parseID(t, out)

	// An independent confirm (session s2) activates it.
	out, errb, code := runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "same failure elsewhere",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("want active after independent confirm, got: %s", out)
	}

	// adjust_scope without --scope is an error.
	_, errb, code = runTM(t, dir, "", "observe", id, "adjust_scope", "--session", "s2")
	if code == 0 {
		t.Fatalf("expected error for adjust_scope without --scope")
	}
	if !strings.Contains(errb, "requires --scope") {
		t.Fatalf("want scope error, got: %s", errb)
	}
}
