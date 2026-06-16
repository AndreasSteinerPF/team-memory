package e2e

import (
	"strings"
	"testing"
)

// TestMarkDuplicateFlow exercises the mark_duplicate path end-to-end:
// propose A and B, mark B as a duplicate of A, verify B's status flips
// immediately (no substantiation gate — auto-effect), the canonical reason
// surfaces, default tm list hides B, and the various filters behave.
// prd.md §5.3, §8.2.
func TestMarkDuplicateFlow(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, code := runTM(t, dir, "",
		"propose", "decision",
		"--title", "A kept",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose A exit %d: %s", code, out)
	}
	idA := parseID(t, out)

	out, _, code = runTM(t, dir, "",
		"propose", "decision",
		"--title", "B duplicate",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose B exit %d: %s", code, out)
	}
	idB := parseID(t, out)

	// File mark_duplicate on B naming A — auto-effect (no substantiation).
	out, errb, code := runTM(t, dir, "",
		"observe", idB, "mark_duplicate",
		"--canonical-id", idA,
		"--summary", "same lesson as A",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe mark_duplicate exit %d: %s / %s", code, out, errb)
	}

	// B immediately flips to duplicate; reason names A.
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: duplicate") {
		t.Fatalf("B should be duplicate, got: %s", out)
	}
	if !strings.Contains(out, "duplicate of "+idA) {
		t.Fatalf("B's reason should name A, got: %s", out)
	}

	// A is unaffected.
	out, _, code = runTM(t, dir, "", "show", idA)
	if code != 0 {
		t.Fatalf("show A exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: duplicate") {
		t.Fatalf("A should not be duplicate, got: %s", out)
	}

	// tm list --duplicate surfaces B; default list hides it.
	out, _, code = runTM(t, dir, "", "list", "--duplicate")
	if code != 0 {
		t.Fatalf("list --duplicate exit %d: %s", code, out)
	}
	if !strings.Contains(out, idB) {
		t.Fatalf("B should be in --duplicate, got: %s", out)
	}
	out, _, code = runTM(t, dir, "", "list")
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, out)
	}
	if strings.Contains(out, idB) {
		t.Fatalf("default list should hide duplicate B, got: %s", out)
	}

	// A later confirm on B resolves the duplicate (per unresolved() rule):
	// the mark_duplicate observation is older than this confirm, so B is no
	// longer "marked as duplicate, not since reconfirmed."
	out, errb, code = runTM(t, dir, "",
		"observe", idB, "confirm",
		"--summary", "actually this is a distinct lesson",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe confirm exit %d: %s / %s", code, out, errb)
	}
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: duplicate") {
		t.Fatalf("B's duplicate should be resolved by a later confirm, got: %s", out)
	}
}
