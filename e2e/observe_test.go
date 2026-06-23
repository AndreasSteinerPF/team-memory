package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserveConfirmActivates(t *testing.T) {
	dir := newGitRepo(t)
	runTM(t, dir, "", "init")

	// Propose a medium-risk failed_attempt (session s1) → provisional.
	out, _, code := runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "rollback needs downgrade tests",
		"--scope", "billing/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s", code, out)
	}
	if !strings.Contains(out, "status: provisional") {
		t.Fatalf("want provisional, got: %s", out)
	}
	id := parseID(t, out)

	// An independent confirm (session s2) activates it.
	out, errb, code := runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "same failure elsewhere",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("observe exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("want active after independent confirm, got: %s", out)
	}

	// adjust_scope without --scope is an error.
	_, errb, code = runTM(t, dir, "", "observe", id, "adjust_scope", "--session", "s2")
	if code == 0 {
		t.Fatalf("expected error for adjust_scope without --scope")
	}
	if !strings.Contains(errb, "requires --scope") {
		t.Fatalf("want scope error, got: %s", errb)
	}
}

func TestDifferentActorIndependenceRequiresDifferentEmail(t *testing.T) {
	dir := newGitRepo(t)
	out, errb, code := runTM(t, dir, "", "init")
	if code != 0 {
		t.Fatalf("init exit %d: %s / %s", code, out, errb)
	}
	enableDifferentActorPolicy(t, dir)

	gitExec(t, dir, "config", "user.email", "dev@example.com")
	out, errb, code = runTM(t, dir, "",
		"propose", "failed_attempt",
		"--title", "rollback needs downgrade tests",
		"--scope", "billing/**",
		"--session", "s1",
	)
	if code != 0 {
		t.Fatalf("propose exit %d: %s / %s", code, out, errb)
	}
	id := parseID(t, out)
	memYAML := gitExec(t, dir, "cat-file", "-p", "teammemory:memories/"+id+".yaml")
	if !strings.Contains(memYAML, "email: dev@example.com") {
		t.Fatalf("memory actor should include git email, got:\n%s", memYAML)
	}

	out, errb, code = runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "same person different session",
		"--session", "s2",
	)
	if code != 0 {
		t.Fatalf("same-email observe exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "status: provisional") {
		t.Fatalf("same git email must not count as independent under different_actor, got:\n%s", out)
	}
	if grep := gitExec(t, dir, "grep", "-n", "email: dev@example.com", "teammemory", "--", "observations/"); grep == "" {
		t.Fatal("observation actor should include git email")
	}

	gitExec(t, dir, "config", "user.email", "reviewer@example.com")
	out, errb, code = runTM(t, dir, "",
		"observe", id, "confirm",
		"--summary", "different person reproduced it",
		"--session", "s3",
	)
	if code != 0 {
		t.Fatalf("different-email observe exit %d: %s / %s", code, out, errb)
	}
	if !strings.Contains(out, "status: active") {
		t.Fatalf("different git email should activate under different_actor, got:\n%s", out)
	}
}

func enableDifferentActorPolicy(t *testing.T, dir string) {
	t.Helper()
	policy := gitExec(t, dir, "cat-file", "-p", "teammemory:policy.yaml")
	policy = strings.Replace(policy, "independence: different_session", "independence: different_actor", 1)
	parent := gitExec(t, dir, "rev-parse", "refs/heads/teammemory")
	blob := gitExecInput(t, dir, []byte(policy), "hash-object", "-w", "--stdin")
	indexFile := filepath.Join(t.TempDir(), "index")
	gitExecEnv(t, dir, []string{"GIT_INDEX_FILE=" + indexFile}, "read-tree", parent)
	gitExecEnv(t, dir, []string{"GIT_INDEX_FILE=" + indexFile}, "update-index", "--add", "--cacheinfo", "100644,"+blob+",policy.yaml")
	tree := gitExecEnv(t, dir, []string{"GIT_INDEX_FILE=" + indexFile}, "write-tree")
	commit := gitExec(t, dir, "commit-tree", tree, "-p", parent, "-m", "test: enable different_actor policy")
	gitExec(t, dir, "update-ref", "refs/heads/teammemory", commit)
}

func gitExecInput(t *testing.T, dir string, stdin []byte, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdin = strings.NewReader(string(stdin))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitExecEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
