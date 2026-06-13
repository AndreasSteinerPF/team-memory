package e2e

import (
	"strings"
	"testing"
)

// wireToOrigin creates a fresh working clone (its own git identity) and points
// its "origin" remote at the shared bare repo. Each clone runs `tm init`
// independently, so the two ledgers start from *unrelated* orphan roots — the
// hardest union-merge case, and exactly what two teammates each running
// `tm init` produce in practice.
func wireToOrigin(t *testing.T, origin string) string {
	t.Helper()
	dir := newGitRepo(t)
	gitExec(t, dir, "remote", "add", "origin", origin)
	return dir
}

// syncOK runs `tm sync` against dir (ledgerRemote defaults to "origin", which
// both clones are wired to) and asserts only that it succeeded. The exact
// SyncResult action for a clone that is ahead of the remote is nondeterministic
// here: `propose`/`observe` fire a best-effort background push, so by the time
// the explicit sync runs the remote may already carry our tip ("up-to-date")
// or not ("created-remote"/"pushed"). Convergence, asserted via list/show, is
// the property that matters — not which branch of Sync produced it.
func syncOK(t *testing.T, dir string) {
	t.Helper()
	if out, errb, code := runTM(t, dir, "", "sync"); code != 0 {
		t.Fatalf("sync exit %d: %s / %s", code, out, errb)
	}
}

// mustMerge runs `tm sync` and requires the union-merge branch specifically.
// A clone that is behind the remote with an *unrelated* history can only
// reconcile via union-merge (its own non-fast-forward background push is
// rejected and leaves the remote untouched), so this action is deterministic
// and is the direct proof that the conflict-free divergence path executed.
func mustMerge(t *testing.T, dir string) {
	t.Helper()
	out, errb, code := runTM(t, dir, "", "sync")
	if code != 0 {
		t.Fatalf("sync exit %d: %s / %s", code, out, errb)
	}
	if got := strings.TrimSpace(out); got != "sync: merged" {
		t.Fatalf("sync action = %q, want %q (divergent union-merge)", got, "sync: merged")
	}
}

// TestSyncUnionMergeAcrossClonesCLI is the end-to-end proof of TeamMemory's
// headline team property (prd.md §7.2/§7.4): two clones propose concurrently
// against a shared remote and converge through the `tm` CLI with no conflict —
// both records survive, visible to both clones via `tm list`.
//
// This complements the library-level ledger.TestTwoCloneConcurrentSyncConverges:
// here each clone runs its own `tm init` (unrelated orphan histories, so the
// merge has no common ancestor), and the assertions go through the CLI + index
// replay rather than the ledger API directly.
func TestSyncUnionMergeAcrossClonesCLI(t *testing.T) {
	origin := t.TempDir()
	gitExec(t, origin, "init", "-q", "--bare", "-b", "main")

	cloneA := wireToOrigin(t, origin)
	cloneB := wireToOrigin(t, origin)

	// Clone A initializes its ledger, proposes a memory, and seeds the remote.
	if _, errb, code := runTM(t, cloneA, "", "init"); code != 0 {
		t.Fatalf("A init: %s", errb)
	}
	outA, _, code := runTM(t, cloneA, "",
		"propose", "decision",
		"--title", "Use ULIDs for record ids",
		"--scope", "docs/**",
		"--session", "a1",
	)
	if code != 0 {
		t.Fatalf("A propose exit %d: %s", code, outA)
	}
	idA := parseID(t, outA)
	syncOK(t, cloneA) // seed the remote with A's ledger

	// Clone B initializes its OWN orphan ledger (unrelated history) and proposes
	// a different memory — no sync between the two proposals.
	if _, errb, code := runTM(t, cloneB, "", "init"); code != 0 {
		t.Fatalf("B init: %s", errb)
	}
	outB, _, code := runTM(t, cloneB, "",
		"propose", "failed_attempt",
		"--title", "Billing migrations need downgrade tests",
		"--scope", "billing/migrations/**",
		"--session", "b1",
	)
	if code != 0 {
		t.Fatalf("B propose exit %d: %s", code, outB)
	}
	idB := parseID(t, outB)

	// B's first sync sees A's tip. The histories diverge with no common ancestor,
	// so this is the union-merge path: no textual conflict, both records kept.
	mustMerge(t, cloneB)

	// A pulls the merged result and converges on the same tip.
	syncOK(t, cloneA)

	// Both clones must now list BOTH memories — convergent and conflict-free.
	for _, c := range []struct{ name, dir string }{{"A", cloneA}, {"B", cloneB}} {
		out, errb, code := runTM(t, c.dir, "", "list")
		if code != 0 {
			t.Fatalf("clone %s list exit %d: %s / %s", c.name, code, out, errb)
		}
		if !strings.Contains(out, idA) || !strings.Contains(out, idB) {
			t.Fatalf("clone %s missing a record after union-merge:\nwant both %s and %s\ngot:\n%s",
				c.name, idA, idB, out)
		}
	}
}

// TestCrossCloneConfirmationActivatesViaSync proves the feature the whole design
// exists to deliver: an *independent* confirmation from a different clone (and a
// different session) activates a provisional memory once the ledgers reconcile.
// Validation crossing the clone boundary is the differentiator — a single-clone
// confirm (TestObserveConfirmActivates) does not exercise it.
func TestCrossCloneConfirmationActivatesViaSync(t *testing.T) {
	origin := t.TempDir()
	gitExec(t, origin, "init", "-q", "--bare", "-b", "main")

	cloneA := wireToOrigin(t, origin)
	cloneB := wireToOrigin(t, origin)

	// A proposes a provisional memory and seeds the remote.
	if _, errb, code := runTM(t, cloneA, "", "init"); code != 0 {
		t.Fatalf("A init: %s", errb)
	}
	outA, _, code := runTM(t, cloneA, "",
		"propose", "failed_attempt",
		"--title", "rollback needs downgrade tests",
		"--scope", "billing/**",
		"--session", "a1",
	)
	if code != 0 {
		t.Fatalf("A propose exit %d: %s", code, outA)
	}
	if !strings.Contains(outA, "status: provisional") {
		t.Fatalf("want provisional on propose, got: %s", outA)
	}
	id := parseID(t, outA)
	syncOK(t, cloneA) // seed the remote with A's provisional memory

	// B starts its own orphan ledger, then reconciles to obtain A's memory.
	// (Whether this fast-forwards or union-merges depends on whether the two
	// independent init commits happened to hash identically — the merge path
	// itself is proven deterministically in TestSyncUnionMergeAcrossClonesCLI;
	// here we only need B to end up holding A's record.)
	if _, errb, code := runTM(t, cloneB, "", "init"); code != 0 {
		t.Fatalf("B init: %s", errb)
	}
	syncOK(t, cloneB) // pull in A's record

	// B independently confirms the memory (different clone, different session).
	// One independent confirm activates it (mirrors TestObserveConfirmActivates).
	outB, errb, code := runTM(t, cloneB, "",
		"observe", id, "confirm",
		"--summary", "same rollback failure reproduced on another branch",
		"--session", "b1",
	)
	if code != 0 {
		t.Fatalf("B observe exit %d: %s / %s", code, outB, errb)
	}
	if !strings.Contains(outB, "status: active") {
		t.Fatalf("want active after independent cross-clone confirm, got: %s", outB)
	}
	syncOK(t, cloneB) // publish B's confirmation to the remote

	// A reconciles and must now see its own memory activated by B's confirmation.
	syncOK(t, cloneA)
	outShow, errb, code := runTM(t, cloneA, "", "show", id)
	if code != 0 {
		t.Fatalf("A show exit %d: %s / %s", code, outShow, errb)
	}
	if !strings.Contains(outShow, "active") {
		t.Fatalf("clone A should see the memory active after cross-clone confirm:\n%s", outShow)
	}
	if !strings.Contains(outShow, "independent confirms: 1") {
		t.Fatalf("clone A should record one independent confirm:\n%s", outShow)
	}
}
