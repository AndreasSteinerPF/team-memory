package ledger

import (
	"errors"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestIsRetryablePushError(t *testing.T) {
	// The exact error TeamMemory's Windows CI hit on a concurrent create race
	// (propose's background push vs. an explicit sync push, both creating the ref).
	ci := errors.New("git push origin refs/heads/teammemory:refs/heads/teammemory: " +
		"exit status 1: remote: error: cannot lock ref 'refs/heads/teammemory': " +
		"reference already exists\nerror: failed to push some refs to '...'")
	if !isRetryablePushError(ci) {
		t.Error("CI lock-race error should be retryable")
	}
	if !isRetryablePushError(errors.New("Updates were rejected; fetch first")) {
		t.Error("non-fast-forward / fetch-first should be retryable")
	}
	if isRetryablePushError(errors.New("fatal: Authentication failed for 'https://...'")) {
		t.Error("auth failure must NOT be retryable")
	}
	if isRetryablePushError(nil) {
		t.Error("nil is not retryable")
	}
}

// TestSyncRetriesThenSucceedsOnLostPushRace simulates the real failure mode: the
// first push loses a concurrent create race, the second succeeds. Sync must
// re-reconcile and converge rather than surfacing the error.
func TestSyncRetriesThenSucceedsOnLostPushRace(t *testing.T) {
	origin := t.TempDir()
	gitRun(t, origin, "init", "-q", "--bare")

	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "t@e")
	gitRun(t, dir, "config", "user.name", "t")
	gitRun(t, dir, "remote", "add", "origin", origin)

	l, err := Open(dir, "teammemory")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "x",
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s"},
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	realPush := l.push
	l.pushFn = func(remote string) error {
		calls++
		if calls == 1 {
			return errors.New("git push: exit status 1: remote: error: " +
				"cannot lock ref 'refs/heads/teammemory': reference already exists")
		}
		return realPush(remote)
	}

	res, err := l.Sync("origin")
	if err != nil {
		t.Fatalf("Sync should recover from a lost push race, got: %v", err)
	}
	if calls < 2 {
		t.Fatalf("expected a retry (≥2 push attempts), got %d", calls)
	}
	if res.Action != "created-remote" {
		t.Fatalf("action = %q, want created-remote", res.Action)
	}
	if got := gitRun(t, origin, "ls-tree", "-r", "--name-only", "refs/heads/teammemory"); !strings.Contains(got, "memories/") {
		t.Fatalf("remote missing memories after sync: %q", got)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git.Runner{Dir: dir}.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}
