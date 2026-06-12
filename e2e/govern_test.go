package e2e

import (
	"strings"
	"testing"
)

func TestApproveRaisesEnforcementAndRejectKills(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, _ := runTM(t, dir, "",
		"propose", "decision", "--title", "policy decision", "--scope", "docs/**", "--session", "s1")
	id := parseID(t, out)

	// approve to requirement + high confidence.
	out, errb, code := runTM(t, dir, "",
		"approve", id, "--enforcement", "requirement", "--confidence", "high")
	if code != 0 {
		t.Fatalf("approve exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "enforcement: requirement") || !strings.Contains(out, "confidence: high") {
		t.Fatalf("want requirement+high, got: %s", out)
	}

	// reject another memory → rejected (terminal).
	out, _, _ = runTM(t, dir, "",
		"propose", "decision", "--title", "bad idea", "--scope", "docs/**", "--session", "s1")
	id2 := parseID(t, out)
	out, _, code = runTM(t, dir, "", "reject", id2, "--summary", "wrong")
	if code != 0 {
		t.Fatalf("reject exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: rejected") {
		t.Fatalf("want rejected, got: %s", out)
	}
}
