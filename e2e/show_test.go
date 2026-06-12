package e2e

import (
	"strings"
	"testing"
)

func TestShowRendersEnvelopeAndState(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "billing/migrations/m.sql", "v1")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	runTM(t, dir, "", "init")

	out, _, _ := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "downgrade tests required",
		"--summary", "rollback failed",
		"--guidance", "add downgrade tests",
		"--scope", "billing/migrations/**",
		"--anchor", "billing/migrations/m.sql@HEAD",
		"--session", "s1",
	)
	id := parseID(t, out)

	out, errb, code := runTM(t, dir, "", "show", id)
	if code != 0 {
		t.Fatalf("show exit %d: %s / %s", code, out, errb)
	}
	for _, want := range []string{
		id, "downgrade tests required", "guidance: add downgrade tests",
		"scope: billing/migrations/**", "status: provisional", "risk: high",
		"anchor: billing/migrations/m.sql @",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	// Unknown id errors.
	_, _, code = runTM(t, dir, "", "show", "01ZZZZZZZZZZZZZZZZZZZZZZZZZ")
	if code == 0 {
		t.Fatalf("expected error for unknown id")
	}
}
