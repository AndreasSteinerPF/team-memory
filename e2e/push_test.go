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

	// The push is detached; poll the bare remote until the memory record
	// arrives. (Init may have already seeded refs/heads/teammemory with just
	// the policy commit, so polling for branch existence alone is insufficient
	// — we need the propose's background push to land.)
	deadline := time.Now().Add(15 * time.Second)
	var out []byte
	for {
		o, err := exec.Command("git", "-C", bare, "ls-tree", "-r", "--name-only",
			"refs/heads/teammemory").Output()
		if err == nil && strings.Contains(string(o), "memories/") {
			out = o
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("ledger branch never arrived on the remote: %v", err)
			}
			t.Fatalf("ledger branch arrived but has no memory record:\n%s", o)
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = out
}
