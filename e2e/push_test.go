package e2e

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestProposeTriggersBackgroundPush covers prd.md §7.4: propose pushes the
// ledger branch to the configured remote best-effort in the background.
func TestProposeTriggersBackgroundPush(t *testing.T) {
	bare := t.TempDir()
	gitExec(t, bare, "init", "-q", "--bare", "-b", "main")

	dir := newGitRepo(t)
	writeFile(t, dir, "billing/migrations/seed.sql", "create table t (id int);")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")
	gitExec(t, dir, "remote", "add", "origin", bare)

	runTM(t, dir, "", "init")
	if _, errOut, code := runTM(t, dir, "", "propose", "failed_attempt",
		"--title", "Billing migrations require downgrade-path tests",
		"--scope", "billing/migrations/**"); code != 0 {
		t.Fatalf("propose failed: %s", errOut)
	}

	// The push is detached; poll the bare remote until the branch arrives.
	deadline := time.Now().Add(15 * time.Second)
	for {
		if exec.Command("git", "-C", bare, "rev-parse", "--verify", "--quiet",
			"refs/heads/teammemory").Run() == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ledger branch never arrived on the remote")
		}
		time.Sleep(100 * time.Millisecond)
	}

	out, err := exec.Command("git", "-C", bare, "ls-tree", "-r", "--name-only",
		"refs/heads/teammemory").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "memories/") {
		t.Fatalf("pushed tree has no memory record:\n%s", out)
	}
}
