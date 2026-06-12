package e2e

import "testing"

// TestInitPersistsSeparateRemote covers prd.md §7.1 separate-remote mode: the
// --remote value is stored as git config tm.remote, and a flagless `tm sync`
// uses it as the push/fetch target.
func TestInitPersistsSeparateRemote(t *testing.T) {
	bare := t.TempDir()
	gitExec(t, bare, "init", "-q", "--bare", "-b", "main")

	dir := newGitRepo(t)
	writeFile(t, dir, "a.txt", "seed")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")

	if _, errOut, code := runTM(t, dir, "", "init", "--remote", bare); code != 0 {
		t.Fatalf("init --remote failed: %s", errOut)
	}

	if got := gitExec(t, dir, "config", "--get", "tm.remote"); got != bare {
		t.Fatalf("tm.remote = %q, want %q", got, bare)
	}

	// Flagless sync must push the ledger branch to the configured remote.
	if _, errOut, code := runTM(t, dir, "", "sync"); code != 0 {
		t.Fatalf("sync failed: %s", errOut)
	}
	gitExec(t, bare, "rev-parse", "--verify", "refs/heads/teammemory") // fails test if absent

	// An explicit --remote must override the stored tm.remote config.
	bare2 := t.TempDir()
	gitExec(t, bare2, "init", "-q", "--bare", "-b", "main")
	if _, errOut, code := runTM(t, dir, "", "sync", "--remote", bare2); code != 0 {
		t.Fatalf("sync --remote override failed: %s", errOut)
	}
	gitExec(t, bare2, "rev-parse", "--verify", "refs/heads/teammemory") // override target got the branch
}
