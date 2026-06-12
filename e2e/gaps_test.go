package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProposeDuplicateWarning verifies that proposing a memory whose title
// matches an existing memory prints a warning to stderr suggesting confirmation
// instead of a duplicate (prd.md §15 spam mitigation).
func TestProposeDuplicateWarning(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	// First proposal creates a memory.
	out, _, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "Billing migrations require downgrade tests",
		"--scope", "billing/migrations/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("first propose exit %d: %s", code, out)
	}
	id := parseID(t, out)

	// Second proposal with a similar title should warn on stderr and still succeed.
	out, errb, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "Billing migrations require downgrade path tests",
		"--scope", "billing/migrations/**",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("second propose exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(errb, "similar memories already exist") {
		t.Fatalf("expected duplicate warning on stderr; got errb=%q", errb)
	}
	if !strings.Contains(errb, id) {
		t.Fatalf("warning should name the existing memory id %s; got errb=%q", id, errb)
	}
	// stdout still has the new ID — propose succeeded.
	newID := parseID(t, out)
	if newID == id {
		t.Fatalf("second propose should produce a new id, got same %s", id)
	}
}

// TestShowPendingScopeAdjustment verifies that an unsubstantiated broadening
// adjust_scope observation is shown as "pending" in `tm show` (prd.md §8.5).
func TestShowPendingScopeAdjustment(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	out, _, code := runTM(t, dir, "",
		"propose", "fragile_area",
		"--title", "Payments module breaks often",
		"--scope", "payments/core/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s", code, out)
	}
	id := parseID(t, out)

	// Broadening adjust_scope: suggested scope is wider than the original.
	out, errb, code := runTM(t, dir, "",
		"observe", id, "adjust_scope",
		"--scope", "payments/**",
		"--summary", "breakage seen across all of payments",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe exit %d: %s / %s", code, out, errb)
	}

	out, errb, code = runTM(t, dir, "", "show", id)
	if code != 0 {
		t.Fatalf("show exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "pending scope adjustments") {
		t.Fatalf("show should report pending broadening; got:\n%s", out)
	}
	if !strings.Contains(out, "payments/**") {
		t.Fatalf("show should name the pending suggested scope; got:\n%s", out)
	}

	// After a human approve the broadening is substantiated → no longer pending.
	runTM(t, dir, "", "approve", id, "--enforcement", "warning", "--confidence", "medium")
	out, _, _ = runTM(t, dir, "", "show", id)
	if strings.Contains(out, "pending scope adjustments") {
		t.Fatalf("show should not report pending after approve; got:\n%s", out)
	}
}

// TestCheckActionWritesFetchStamp verifies that check-action writes a
// last_fetch timestamp file under .git/tm/ (prd.md §7.4 background fetch).
func TestCheckActionWritesFetchStamp(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "pkg/foo.go", "package foo")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	runTM(t, dir, "", "init")

	// Obtain the gitDir path.
	gitDir := filepath.Join(dir, ".git")

	stampFile := filepath.Join(gitDir, "tm", "last_fetch")
	if _, err := os.Stat(stampFile); err == nil {
		t.Fatalf("stamp file should not exist before first check-action")
	}

	runTM(t, dir, "", "check-action", "--path", "pkg/foo.go")

	data, err := os.ReadFile(stampFile)
	if err != nil {
		t.Fatalf("last_fetch stamp not written after check-action: %v", err)
	}
	stamp := strings.TrimSpace(string(data))
	if stamp == "" {
		t.Fatalf("last_fetch stamp is empty")
	}

	// Running again immediately should NOT update the stamp (still fresh).
	runTM(t, dir, "", "check-action", "--path", "pkg/foo.go")
	data2, _ := os.ReadFile(stampFile)
	if strings.TrimSpace(string(data2)) != stamp {
		t.Fatalf("stamp changed on re-run within interval; got %q, want %q",
			strings.TrimSpace(string(data2)), stamp)
	}
}
