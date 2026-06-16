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

// TestMarkDuplicateCycleWarnsButDoesNotBlock pins the warn-not-block contract
// for one-hop duplicate cycles (prd.md §8.2). Propose A and B; mark A as a
// duplicate of B (A->B); then mark B as a duplicate of A (B->A). The second
// observation closes a cycle: the CLI must succeed (exit 0), emit a stderr
// warning naming "duplicate cycle", and end with both memories in
// `tm list --duplicate`.
func TestMarkDuplicateCycleWarnsButDoesNotBlock(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, code := runTM(t, dir, "",
		"propose", "decision",
		"--title", "A first",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose A exit %d: %s", code, out)
	}
	idA := parseID(t, out)

	out, _, code = runTM(t, dir, "",
		"propose", "decision",
		"--title", "B second",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose B exit %d: %s", code, out)
	}
	idB := parseID(t, out)

	// First leg: A is a duplicate of B (A -> B). No cycle yet.
	out, errb, code := runTM(t, dir, "",
		"observe", idA, "mark_duplicate",
		"--canonical-id", idB,
		"--summary", "A duplicates B",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe A->B exit %d: %s / %s", code, out, errb)
	}
	if strings.Contains(errb, "duplicate cycle") {
		t.Fatalf("first leg must not emit a cycle warning, got: %s", errb)
	}

	// Second leg: B is a duplicate of A (B -> A). Closes the cycle.
	out, errb, code = runTM(t, dir, "",
		"observe", idB, "mark_duplicate",
		"--canonical-id", idA,
		"--summary", "B duplicates A",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe B->A exit %d (cycle should warn, not block): stdout=%s stderr=%s", code, out, errb)
	}
	if !strings.Contains(errb, "duplicate cycle") {
		t.Fatalf("cycle warning missing from stderr, got: %s", errb)
	}

	// Both memories appear in --duplicate after the cycle.
	out, _, code = runTM(t, dir, "", "list", "--duplicate")
	if code != 0 {
		t.Fatalf("list --duplicate exit %d: %s", code, out)
	}
	if !strings.Contains(out, idA) {
		t.Fatalf("A should appear in --duplicate after cycle, got: %s", out)
	}
	if !strings.Contains(out, idB) {
		t.Fatalf("B should appear in --duplicate after cycle, got: %s", out)
	}
}

// TestMarkDuplicateThreeHopCycleWarns pins multi-hop cycle detection. The
// one-hop detector caught A↔B; the chain walker must also catch A→B→C→A.
// Without it, three memories silently disappear from default retrieval with
// no warning.
func TestMarkDuplicateThreeHopCycleWarns(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	propose := func(title string) string {
		out, _, code := runTM(t, dir, "",
			"propose", "decision",
			"--title", title,
			"--scope", "docs/**",
			"--session", "s1",
		)
		if code != 0 {
			t.Fatalf("propose %s exit %d: %s", title, code, out)
		}
		return parseID(t, out)
	}
	idA := propose("A")
	idB := propose("B")
	idC := propose("C")

	// A→B and B→C: no cycle yet, no warning.
	for _, leg := range []struct{ from, to string }{
		{idA, idB},
		{idB, idC},
	} {
		_, errb, code := runTM(t, dir, "",
			"observe", leg.from, "mark_duplicate",
			"--canonical-id", leg.to,
			"--summary", "chain leg",
			"--session", "s1",
		)
		if code != 0 {
			t.Fatalf("observe %s->%s exit %d: %s", leg.from, leg.to, code, errb)
		}
		if strings.Contains(errb, "duplicate cycle") {
			t.Fatalf("chain leg %s->%s should not warn yet, got: %s", leg.from, leg.to, errb)
		}
	}

	// C→A: closes the 3-cycle. Must warn but not block.
	_, errb, code := runTM(t, dir, "",
		"observe", idC, "mark_duplicate",
		"--canonical-id", idA,
		"--summary", "closing leg",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe C->A (closing 3-cycle) should not block, exit %d: %s", code, errb)
	}
	if !strings.Contains(errb, "duplicate cycle") {
		t.Fatalf("3-cycle closing leg must warn on stderr, got: %s", errb)
	}
}

// TestMarkDuplicateRevertedWhenCanonicalRejected pins R-N2 orphan revival
// (prd.md §8.5): when the canonical is rejected, the duplicate memory
// reverts from status=duplicate to its un-orphaned status.
func TestMarkDuplicateRevertedWhenCanonicalRejected(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	propose := func(title string) string {
		out, _, code := runTM(t, dir, "",
			"propose", "decision",
			"--title", title,
			"--scope", "docs/**",
			"--session", "s1",
		)
		if code != 0 {
			t.Fatalf("propose %s exit %d: %s", title, code, out)
		}
		return parseID(t, out)
	}
	idA := propose("A canonical")
	idB := propose("B duplicate")

	// Mark B as a duplicate of A.
	if _, errb, code := runTM(t, dir, "",
		"observe", idB, "mark_duplicate",
		"--canonical-id", idA,
		"--summary", "B duplicates A",
		"--session", "s1",
	); code != 0 {
		t.Fatalf("mark_duplicate exit %d: %s", code, errb)
	}
	out, _, code := runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: duplicate") {
		t.Fatalf("B should be duplicate before reject, got: %s", out)
	}

	// Reject A. Under orphan revival, B should revert.
	if _, errb, code := runTM(t, dir, "",
		"reject", idA, "--summary", "A is wrong",
	); code != 0 {
		t.Fatalf("reject A exit %d: %s", code, errb)
	}
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: duplicate") {
		t.Fatalf("B should revert from duplicate after canonical A is rejected, got: %s", out)
	}
}

// TestMarkDuplicateRevertedWhenCanonicalMarkedStale pins R-N2 for the
// mark_stale path (prd.md §8.5): a non-rejected canonical that goes stale
// also reverts the duplicate, mirroring the reject case. This pins the
// incremental Update() fan-out path for mark_stale on the canonical.
func TestMarkDuplicateRevertedWhenCanonicalMarkedStale(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	propose := func(title string) string {
		out, _, code := runTM(t, dir, "",
			"propose", "decision",
			"--title", title,
			"--scope", "docs/**",
			"--session", "s1",
		)
		if code != 0 {
			t.Fatalf("propose %s exit %d: %s", title, code, out)
		}
		return parseID(t, out)
	}
	idA := propose("A canonical")
	idB := propose("B duplicate")

	// B is a duplicate of A.
	if _, errb, code := runTM(t, dir, "",
		"observe", idB, "mark_duplicate",
		"--canonical-id", idA,
		"--summary", "B duplicates A",
		"--session", "s1",
	); code != 0 {
		t.Fatalf("mark_duplicate exit %d: %s", code, errb)
	}
	out, _, code := runTM(t, dir, "", "show", idB)
	if code != 0 || !strings.Contains(out, "status: duplicate") {
		t.Fatalf("B should be duplicate, got: %s", out)
	}

	// mark A stale. B should revert under orphan revival.
	if _, errb, code := runTM(t, dir, "",
		"observe", idA, "mark_stale",
		"--summary", "A no longer applies",
		"--session", "s2",
	); code != 0 {
		t.Fatalf("mark_stale A exit %d: %s", code, errb)
	}
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: duplicate") {
		t.Fatalf("B should revert from duplicate after canonical A is marked stale, got: %s", out)
	}
}
