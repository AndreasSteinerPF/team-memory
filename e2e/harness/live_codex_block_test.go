//go:build harness_live

package harness_e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Codex requirement-blocking is verified separately from the other harnesses
// because codex only fires hooks after its repo's hooks.json is TRUSTED ONCE
// interactively, and that trust is keyed to the hooks.json hash — so the repo
// can't be a throwaway temp dir whose hooks we rewrite per run (that's why the
// shared TestLiveRequirementBlock skips codex). Instead:
//
//  1. `TestSetupCodexBlockRepo` (TM_CODEX_BLOCK_REPO=/path) scaffolds a persistent
//     repo with REAL tm hooks (bare `tm` on PATH — so the trust hash is stable)
//     and an active requirement scoped to protected.txt, and writes the memory id
//     to .tm-block-memid.
//  2. You trust it once: `cd /path && codex` → "Trust all and continue" → /quit.
//  3. `TestLiveCodexRequirementBlock` (same env var) drives `codex exec` and
//     asserts protected.txt is not written AND the requirement is surfaced.
//
// Because the hooks call bare `tm`, the test exercises the tm on PATH — run
// `go install ./cmd/tm` first if you want it to reflect uncommitted changes.

const codexBlockMemIDFile = ".tm-block-memid"

func gitInitAt(t *testing.T, repo string) {
	t.Helper()
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func TestSetupCodexBlockRepo(t *testing.T) {
	repo := os.Getenv("TM_CODEX_BLOCK_REPO")
	if repo == "" {
		t.Skip("set TM_CODEX_BLOCK_REPO=/path to scaffold the codex block-test repo")
	}
	gitInitAt(t, repo)
	if code := runInit(repo, "codex"); code != 0 {
		t.Fatalf("tm init --harness codex failed")
	}
	memID := proposeActiveRequirement(t, repo, "protected.txt")
	if err := os.WriteFile(filepath.Join(repo, codexBlockMemIDFile), []byte(memID), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("Scaffolded codex block-test repo at %s (active requirement %s).", repo, memID)
	t.Logf("NEXT (one-time): cd %q && codex  → choose \"Trust all and continue\" → /quit", repo)
	t.Logf("THEN: TM_CODEX_BLOCK_REPO=%q go test -tags harness_live ./e2e/harness/ -run TestLiveCodexRequirementBlock -v", repo)
}

func TestLiveCodexRequirementBlock(t *testing.T) {
	repo := os.Getenv("TM_CODEX_BLOCK_REPO")
	if repo == "" {
		t.Skip("set TM_CODEX_BLOCK_REPO (scaffold via TestSetupCodexBlockRepo, then trust once via `codex`)")
	}
	memBytes, err := os.ReadFile(filepath.Join(repo, codexBlockMemIDFile))
	if err != nil {
		t.Fatalf("missing %s in %s — run TestSetupCodexBlockRepo first: %v", codexBlockMemIDFile, repo, err)
	}
	memID := string(memBytes)
	drv, ok := GetDriver("codex")
	if !ok {
		t.Skip("no codex driver")
	}
	if err := requireCLI(drv); err != nil {
		t.Fatalf("%v", err)
	}
	// Clear any protected.txt left by a prior run so a stale file can't masquerade
	// as a block failure.
	_ = os.Remove(filepath.Join(repo, "protected.txt"))

	ctx, cancel := context.WithTimeout(context.Background(), captureTimeout())
	defer cancel()
	prompt := "Create a file named protected.txt containing the word hello. " +
		"If a hook blocks the write, stop and do not retry."
	if err := driveCLIInRepo(ctx, drv, repo, filepath.Join(t.TempDir(), "unused.jsonl"), prompt); err != nil {
		t.Fatalf("[codex] drive: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo, "protected.txt")); err == nil {
		t.Errorf("[codex] requirement block FAILED: protected.txt was written despite an active requirement (codex may not honor a PreToolUse deny, or --dangerously-bypass-approvals-and-sandbox swallows it)")
	}
	if !journalContains(repo, memID) {
		t.Errorf("[codex] requirement %s not surfaced — the check-action hook may not have fired (repo not trusted? block unverified)", memID)
	}
}
