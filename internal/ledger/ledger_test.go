package ledger_test

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
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

func TestAppendAndReadRoundTrip(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}

	memID, err := l.AppendMemory(model.Memory{
		Type:      model.TypeFailedAttempt,
		Title:     "Billing migrations require downgrade-path tests",
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "claude-code", SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append memory: %v", err)
	}
	if len(memID) != 26 {
		t.Fatalf("expected a ULID id to be assigned, got %q", memID)
	}

	obsID, err := l.AppendObservation(model.Observation{
		Target:    memID,
		Kind:      model.KindConfirm,
		Summary:   "Reproduced on revenue-reporting branch.",
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "codex", SessionID: "s2"},
		CreatedAt: time.Date(2026, 6, 15, 11, 20, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("append observation: %v", err)
	}

	// Read everything back through a freshly opened handle (no in-memory state).
	l2, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	mems, err := l2.Memories()
	if err != nil {
		t.Fatalf("memories: %v", err)
	}
	if len(mems) != 1 || mems[0].ID != memID || mems[0].Title != "Billing migrations require downgrade-path tests" {
		t.Fatalf("unexpected memories: %+v", mems)
	}

	obs, err := l2.Observations()
	if err != nil {
		t.Fatalf("observations: %v", err)
	}
	if len(obs) != 1 || obs[0].ID != obsID || obs[0].Target != memID || obs[0].Kind != model.KindConfirm {
		t.Fatalf("unexpected observations: %+v", obs)
	}

	pol, err := l2.Policy()
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if string(pol) != "retrieval:\n  max_results: 5" {
		t.Fatalf("unexpected policy: %q", string(pol))
	}

	// Working tree still clean after appends.
	if status := mustGit(t, git.Runner{Dir: dir}, "status", "--porcelain"); status != "" {
		t.Fatalf("working tree should be clean, got:\n%s", status)
	}
}
