package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// runTM runs `tm` in-process against repo and returns stdout, stderr, exit code.
func runTM(t *testing.T, repo string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(append([]string{"--repo", repo}, args...), strings.NewReader(""), &out, &errb)
	return out.String(), errb.String(), code
}

// TestShowSurfacesPendingSupersession verifies prd.md §8.5: a supersede
// observation that names B but lacks substantiation must be visible on
// `tm show B` so reviewers can see the unresolved claim.
func TestShowSurfacesPendingSupersession(t *testing.T) {
	repo := initRepo(t)

	// Propose B first, then A.
	outB, errB, code := runTM(t, repo, "propose", "decision", "--title", "B", "--scope", "**")
	if code != 0 {
		t.Fatalf("propose B failed (%d): %s", code, errB)
	}
	idB := strings.TrimSpace(strings.SplitN(outB, "\n", 2)[0])

	outA, errA, code := runTM(t, repo, "propose", "decision", "--title", "A", "--scope", "**")
	if code != 0 {
		t.Fatalf("propose A failed (%d): %s", code, errA)
	}
	idA := strings.TrimSpace(strings.SplitN(outA, "\n", 2)[0])

	// A supersedes B; no confirm/approve yet, so the claim is pending.
	if _, errObs, code := runTM(t, repo, "observe", idA, "supersede",
		"--supersedes", idB, "--summary", "A is the new canonical"); code != 0 {
		t.Fatalf("observe supersede failed (%d): %s", code, errObs)
	}

	// `tm show B` must surface the pending claim.
	showB, errShow, code := runTM(t, repo, "show", idB)
	if code != 0 {
		t.Fatalf("show B failed (%d): %s", code, errShow)
	}
	if !strings.Contains(showB, "pending supersession claims naming this memory:") {
		t.Errorf("show B missing pending header.\noutput:\n%s", showB)
	}
	if !strings.Contains(showB, "by "+idA) {
		t.Errorf("show B missing claimant %q.\noutput:\n%s", idA, showB)
	}

	// `tm show A` is the canonical, not the obsolete — no pending block.
	showA, errShow, code := runTM(t, repo, "show", idA)
	if code != 0 {
		t.Fatalf("show A failed (%d): %s", code, errShow)
	}
	if strings.Contains(showA, "pending supersession claims naming this memory") {
		t.Errorf("show A unexpectedly rendered a pending block.\noutput:\n%s", showA)
	}
}
