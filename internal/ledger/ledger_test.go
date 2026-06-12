package ledger_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
)

const branch = "teammemory"

// newRepo creates a git repo on branch "main" with a committer identity set.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	mustGit(t, r, "init", "-q", "-b", "main")
	mustGit(t, r, "config", "user.email", "test@example.com")
	mustGit(t, r, "config", "user.name", "Test")
	return dir
}

func mustGit(t *testing.T, r git.Runner, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func TestInitCreatesBranchWithPolicyAndLeavesWorkingTreeClean(t *testing.T) {
	dir := newRepo(t)
	r := git.Runner{Dir: dir}

	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if l.Exists() {
		t.Fatal("branch should not exist before Init")
	}

	if err := l.Init([]byte("base_risk:\n  decision: low\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !l.Exists() {
		t.Fatal("branch should exist after Init")
	}

	// policy.yaml is readable from the orphan branch.
	got := mustGit(t, r, "cat-file", "-p", "refs/heads/"+branch+":policy.yaml")
	if got != "base_risk:\n  decision: low" {
		t.Fatalf("unexpected policy content: %q", got)
	}

	// The working tree and the real index are untouched: nothing is staged or
	// modified on the checked-out main branch.
	if status := mustGit(t, r, "status", "--porcelain"); status != "" {
		t.Fatalf("working tree should be clean, got:\n%s", status)
	}

	// Re-initialising an existing ledger is an error.
	if err := l.Init([]byte("x: y\n")); err == nil {
		t.Fatal("expected error re-initialising an existing ledger")
	}
}

func TestOpenRejectsNonRepository(t *testing.T) {
	if _, err := ledger.Open(t.TempDir(), branch); err == nil {
		t.Fatal("expected Open to fail outside a git repository")
	}
}
