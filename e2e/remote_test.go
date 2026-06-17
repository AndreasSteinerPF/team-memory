package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestSeparateRemoteBranchProtectionDiagnosis exercises prd.md §15 / §7.1 end
// to end against a real bare remote that rejects pushes to refs/heads/teammemory
// via a pre-receive hook. It verifies that:
//   - tm init records the rejection (tm.remote is still stored — URL was
//     reachable per ls-remote — and init prints a branch-protection diagnosis).
//   - tm sync prints the same diagnosis on stderr.
//   - tm status surfaces the consecutive-failure warning.
//   - tm doctor flags it under "Recent push failures".
//   - tm remote set <good> + a follow-up tm sync clears the failure and the
//     status warning disappears.
func TestSeparateRemoteBranchProtectionDiagnosis(t *testing.T) {
	// Step 1: code repo with one commit.
	dir := newGitRepo(t)
	writeFile(t, dir, "seed.txt", "seed")
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "-q", "-m", "seed")

	// Step 2: rejecting bare remote.
	badBare := t.TempDir()
	gitExec(t, badBare, "init", "-q", "--bare", "-b", "main")

	// Step 3: pre-receive hook rejects pushes to refs/heads/teammemory with the
	// canonical GH006 stderr that ClassifyPushStderr recognises.
	hook := "#!/bin/sh\n" +
		"while read _ _ ref; do\n" +
		"  if [ \"$ref\" = \"refs/heads/teammemory\" ]; then\n" +
		"    echo 'remote: GH006: Protected branch update failed for refs/heads/teammemory.' >&2\n" +
		"    exit 1\n" +
		"  fi\n" +
		"done\n" +
		"exit 0\n"
	hookPath := filepath.Join(badBare, "hooks", "pre-receive")
	if err := os.WriteFile(hookPath, []byte(hook), 0o755); err != nil {
		t.Fatalf("write pre-receive hook: %v", err)
	}

	// Step 4: tm init --remote <badBare> must succeed (the URL is reachable per
	// ls-remote) and must print the branch-protection diagnosis.
	stdout, stderr, code := runTM(t, dir, "", "init", "--remote", badBare)
	if code != 0 {
		t.Fatalf("init --remote (rejecting) failed: stdout=%s stderr=%s", stdout, stderr)
	}
	if got := gitExec(t, dir, "config", "--get", "tm.remote"); got != badBare {
		t.Fatalf("tm.remote = %q, want %q", got, badBare)
	}
	initOut := stdout + stderr
	if !strings.Contains(initOut, "branch protection") {
		t.Fatalf("init output missing 'branch protection' diagnosis:\n%s", initOut)
	}
	if !strings.Contains(initOut, "tm remote set") {
		t.Fatalf("init output missing 'tm remote set' hint:\n%s", initOut)
	}

	// Step 5: tm sync emits the same diagnosis on stderr and bumps consecutive
	// to 2 so tm status will surface a warning.
	stdout, stderr, _ = runTM(t, dir, "", "sync")
	syncOut := stdout + stderr
	if !strings.Contains(syncOut, "protected branch") {
		t.Fatalf("sync stderr missing 'protected branch' diagnosis:\n%s", syncOut)
	}

	// Step 6: tm status warns about consecutive background-push rejections.
	stdout, stderr, code = runTM(t, dir, "", "status")
	if code != 0 {
		t.Fatalf("status failed: rc=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "background pushes to") {
		t.Fatalf("status missing 'background pushes to' warning:\n%s", stdout)
	}
	if !strings.Contains(stdout, "(protected branch)") {
		t.Fatalf("status missing '(protected branch)' label:\n%s", stdout)
	}

	// Step 7: tm doctor reports "Recent push failures" with a fix hint.
	stdout, stderr, _ = runTM(t, dir, "", "doctor")
	doctorOut := stdout + stderr
	if !strings.Contains(doctorOut, "Recent push failures") {
		t.Fatalf("doctor missing 'Recent push failures' line:\n%s", doctorOut)
	}
	if !strings.Contains(doctorOut, "tm remote set") {
		t.Fatalf("doctor missing 'tm remote set' fix hint:\n%s", doctorOut)
	}

	// Step 8: accepting bare remote.
	goodBare := t.TempDir()
	gitExec(t, goodBare, "init", "-q", "--bare", "-b", "main")

	// Step 9: tm remote set <good> repoints tm.remote.
	if stdout, stderr, code := runTM(t, dir, "", "remote", "set", goodBare); code != 0 {
		t.Fatalf("remote set <good> failed: rc=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if got := gitExec(t, dir, "config", "--get", "tm.remote"); got != goodBare {
		t.Fatalf("after remote set: tm.remote = %q, want %q", got, goodBare)
	}

	// Step 10: tm sync now succeeds and the success path clears
	// .git/tm/push_failure.json (env.go OnPushResult).
	stdout, stderr, code = runTM(t, dir, "", "sync")
	if code != 0 {
		t.Fatalf("sync after recovery failed: rc=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	syncOut = stdout + stderr
	if strings.Contains(syncOut, "protected branch") {
		t.Fatalf("recovery sync still reports protected branch:\n%s", syncOut)
	}
	gitExec(t, goodBare, "rev-parse", "--verify", "refs/heads/teammemory") // ledger landed on the good remote

	// Step 11: tm status no longer warns.
	stdout, stderr, code = runTM(t, dir, "", "status")
	if code != 0 {
		t.Fatalf("status after recovery failed: rc=%d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, "background pushes to") {
		t.Fatalf("status still warns after recovery:\n%s", stdout)
	}
}
