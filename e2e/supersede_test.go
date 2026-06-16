package e2e

import (
	"strings"
	"testing"
)

// TestSupersedeFlow exercises the full cross-memory supersession path:
// propose A and B, file a supersede on A naming B (pending), file an
// independent confirm on A (substantiates), and verify B transitions to
// status=superseded with the canonical reason. Pins the user-visible
// behavior of the first cross-memory derive state (prd.md §8.5).
func TestSupersedeFlow(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, code := runTM(t, dir, "",
		"propose", "decision",
		"--title", "A canonical",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose A exit %d: %s", code, out)
	}
	idA := parseID(t, out)

	out, _, code = runTM(t, dir, "",
		"propose", "decision",
		"--title", "B obsolete",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose B exit %d: %s", code, out)
	}
	idB := parseID(t, out)

	// File supersede on A naming B; relationship is pending (no independent
	// confirm or human approve on A yet).
	out, errb, code := runTM(t, dir, "",
		"observe", idA, "supersede",
		"--supersedes", idB,
		"--summary", "A replaces B",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe supersede exit %d: %s / %s", code, out, errb)
	}

	// B is still active — the supersede has not been substantiated.
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: superseded") {
		t.Fatalf("B should not be superseded before substantiation, got: %s", out)
	}

	// B should appear in --pending-supersede; not yet in --superseded.
	out, _, code = runTM(t, dir, "", "list", "--pending-supersede")
	if code != 0 {
		t.Fatalf("list --pending-supersede exit %d: %s", code, out)
	}
	if !strings.Contains(out, idB) {
		t.Fatalf("B should be listed under --pending-supersede, got: %s", out)
	}
	out, _, code = runTM(t, dir, "", "list", "--superseded")
	if code != 0 {
		t.Fatalf("list --superseded exit %d: %s", code, out)
	}
	if strings.Contains(out, idB) {
		t.Fatalf("B should not yet be listed under --superseded, got: %s", out)
	}

	// Independent confirm on A (different session) substantiates the supersede.
	out, errb, code = runTM(t, dir, "",
		"observe", idA, "confirm",
		"--summary", "I hit this elsewhere",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe confirm exit %d: %s / %s", code, out, errb)
	}

	// B is now superseded with the canonical reason pointing at A.
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: superseded") {
		t.Fatalf("B should be superseded after substantiation, got: %s", out)
	}
	if !strings.Contains(out, "superseded by "+idA) {
		t.Fatalf("B's reason should name A as the new canonical, got: %s", out)
	}

	// B now appears in --superseded; pending list no longer carries the claim.
	out, _, code = runTM(t, dir, "", "list", "--superseded")
	if code != 0 {
		t.Fatalf("list --superseded exit %d: %s", code, out)
	}
	if !strings.Contains(out, idB) {
		t.Fatalf("B should be listed under --superseded after substantiation, got: %s", out)
	}
	out, _, code = runTM(t, dir, "", "list", "--pending-supersede")
	if code != 0 {
		t.Fatalf("list --pending-supersede exit %d: %s", code, out)
	}
	if strings.Contains(out, idB) {
		t.Fatalf("B should no longer be in --pending-supersede once substantiated, got: %s", out)
	}

	// Default list excludes superseded (consistent with retrieve §11.4).
	out, _, code = runTM(t, dir, "", "list")
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, out)
	}
	if strings.Contains(out, idB) {
		t.Fatalf("default tm list should hide superseded B, got: %s", out)
	}
}

// TestSupersedeOnStaleARevivedCascade pins the operator-visible "stale-revival
// cascade" behavior: a supersede observation filed on A while A is later
// marked stale is still silently substantiated when A is subsequently revived
// by an independent confirm. v1 derive does not gate substantiation on A's
// intermediate status (see internal/derive/supersede.go::supersedeSubstantiated)
// — any later approve/independent-confirm on A substantiates the supersede,
// regardless of any stale interlude. If we ever want this NOT to happen, the
// fix is to gate substantiation on A's current status; for v1 the cascade is
// the contract (prd.md §8.2, §8.5). This test breaks loudly if that contract
// changes.
func TestSupersedeOnStaleARevivedCascade(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, code := runTM(t, dir, "",
		"propose", "decision",
		"--title", "A canonical",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose A exit %d: %s", code, out)
	}
	idA := parseID(t, out)

	out, _, code = runTM(t, dir, "",
		"propose", "decision",
		"--title", "B obsolete",
		"--scope", "docs/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose B exit %d: %s", code, out)
	}
	idB := parseID(t, out)

	// File supersede on A naming B (session s1). A and B both still pending
	// substantiation; B not yet superseded.
	out, errb, code := runTM(t, dir, "",
		"observe", idA, "supersede",
		"--supersedes", idB,
		"--summary", "A replaces B",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe supersede exit %d: %s / %s", code, out, errb)
	}

	// Mark A stale. A's status flips to stale; supersede claim remains pending.
	out, errb, code = runTM(t, dir, "",
		"observe", idA, "mark_stale",
		"--summary", "A no longer applies",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe mark_stale exit %d: %s / %s", code, out, errb)
	}
	out, _, code = runTM(t, dir, "", "show", idA)
	if code != 0 {
		t.Fatalf("show A exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: stale") {
		t.Fatalf("A should be stale after mark_stale, got: %s", out)
	}
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: superseded") {
		t.Fatalf("B should NOT be superseded while supersede is unsubstantiated, got: %s", out)
	}

	// Independent confirm on A (different session). This post-dates the
	// mark_stale (so A revives to active) AND post-dates the supersede with an
	// independent actor — so it substantiates the supersede on B. The cascade:
	// the revival event simultaneously substantiates the supersede claim.
	out, errb, code = runTM(t, dir, "",
		"observe", idA, "confirm",
		"--summary", "A still applies, hit it again",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe confirm exit %d: %s / %s", code, out, errb)
	}

	// A is active again.
	out, _, code = runTM(t, dir, "", "show", idA)
	if code != 0 {
		t.Fatalf("show A exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("A should be active after revival confirm, got: %s", out)
	}

	// B has cascaded to superseded — the v1 contract.
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: superseded") {
		t.Fatalf("B should be superseded via stale-revival cascade, got: %s", out)
	}
	if !strings.Contains(out, "superseded by "+idA) {
		t.Fatalf("B's reason should name A as canonical, got: %s", out)
	}
}

// TestSupersedeCycleWarnsButDoesNotBlock pins the warn-not-block contract for
// one-hop supersede cycles (prd.md §8.2, §8.5). Propose A and B; file
// supersede on A naming B (A supersedes B); then file supersede on B naming A
// (B supersedes A). The second observation closes a cycle: the CLI must
// succeed (exit 0) and emit a stderr warning naming "supersede cycle".
func TestSupersedeCycleWarnsButDoesNotBlock(t *testing.T) {
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

	// First leg: file supersede on A naming B (A supersedes B). No cycle.
	out, errb, code := runTM(t, dir, "",
		"observe", idA, "supersede",
		"--supersedes", idB,
		"--summary", "A supersedes B",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("observe A supersedes B exit %d: %s / %s", code, out, errb)
	}
	if strings.Contains(errb, "supersede cycle") {
		t.Fatalf("first leg must not emit a cycle warning, got: %s", errb)
	}

	// Second leg: file supersede on B naming A (B supersedes A). Closes the cycle.
	out, errb, code = runTM(t, dir, "",
		"observe", idB, "supersede",
		"--supersedes", idA,
		"--summary", "B supersedes A",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe B supersedes A exit %d (cycle should warn, not block): stdout=%s stderr=%s", code, out, errb)
	}
	if !strings.Contains(errb, "supersede cycle") {
		t.Fatalf("cycle warning missing from stderr, got: %s", errb)
	}
}

// TestSupersedeRevertedWhenCanonicalRejected pins R-N2 orphan revival
// (prd.md §8.5): once B is superseded by A, rejecting A reverts B from
// status=superseded back to its un-orphaned status.
func TestSupersedeRevertedWhenCanonicalRejected(t *testing.T) {
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
	idB := propose("B obsolete")

	// Supersede on A naming B.
	if _, errb, code := runTM(t, dir, "",
		"observe", idA, "supersede",
		"--supersedes", idB,
		"--summary", "A replaces B",
		"--session", "s1",
	); code != 0 {
		t.Fatalf("supersede exit %d: %s", code, errb)
	}
	// Independent confirm on A substantiates.
	if _, errb, code := runTM(t, dir, "",
		"observe", idA, "confirm",
		"--summary", "I hit this elsewhere",
		"--session", "s2",
	); code != 0 {
		t.Fatalf("confirm exit %d: %s", code, errb)
	}
	out, _, code := runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: superseded") {
		t.Fatalf("B should be superseded before reject, got: %s", out)
	}

	// Reject A. Under orphan revival, B reverts.
	if _, errb, code := runTM(t, dir, "",
		"reject", idA, "--summary", "A is wrong",
	); code != 0 {
		t.Fatalf("reject A exit %d: %s", code, errb)
	}
	out, _, code = runTM(t, dir, "", "show", idB)
	if code != 0 {
		t.Fatalf("show B exit %d: %s", code, out)
	}
	if strings.Contains(out, "status: superseded") {
		t.Fatalf("B should revert from superseded after canonical A is rejected, got: %s", out)
	}
}
