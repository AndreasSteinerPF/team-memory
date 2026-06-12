package ledger_test

import (
	"sort"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// bareRemote creates an empty bare repository to act as the shared origin.
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, git.Runner{Dir: dir}, "init", "-q", "--bare")
	return dir
}

func memoryIDs(t *testing.T, l *ledger.Ledger) []string {
	t.Helper()
	mems, err := l.Memories()
	if err != nil {
		t.Fatalf("memories: %v", err)
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestTwoCloneConcurrentSyncConverges(t *testing.T) {
	origin := bareRemote(t)

	// Clone A: a repo wired to origin, with the ledger initialised and pushed.
	dirA := newRepo(t)
	mustGit(t, git.Runner{Dir: dirA}, "remote", "add", "origin", origin)
	lA, err := ledger.Open(dirA, branch)
	if err != nil {
		t.Fatalf("open A: %v", err)
	}
	if err := lA.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init A: %v", err)
	}
	if _, err := lA.Sync("origin"); err != nil { // creates the branch on origin
		t.Fatalf("A initial sync: %v", err)
	}

	// Clone B: a second repo wired to origin that adopts the ledger branch from
	// origin (this is what a teammate's first sync after cloning does).
	dirB := newRepo(t)
	mustGit(t, git.Runner{Dir: dirB}, "remote", "add", "origin", origin)
	mustGit(t, git.Runner{Dir: dirB}, "fetch", "-q", "origin", branch)
	mustGit(t, git.Runner{Dir: dirB}, "update-ref", "refs/heads/"+branch, "FETCH_HEAD")
	lB, err := ledger.Open(dirB, branch)
	if err != nil {
		t.Fatalf("open B: %v", err)
	}

	// Both clones append a memory concurrently (no sync between the appends).
	idA, err := lA.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "from A",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "sa"},
	})
	if err != nil {
		t.Fatalf("append A: %v", err)
	}
	idB, err := lB.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "from B",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "b", SessionID: "sb"},
	})
	if err != nil {
		t.Fatalf("append B: %v", err)
	}

	// A pushes first (fast-forward over origin's init commit).
	if _, err := lA.Sync("origin"); err != nil {
		t.Fatalf("A sync: %v", err)
	}
	// B has diverged from origin → union-merge then push.
	resB, err := lB.Sync("origin")
	if err != nil {
		t.Fatalf("B sync: %v", err)
	}
	if resB.Action != "merged" {
		t.Fatalf("expected B to union-merge, got %q", resB.Action)
	}
	// A syncs again → fast-forward onto B's merge commit.
	resA, err := lA.Sync("origin")
	if err != nil {
		t.Fatalf("A second sync: %v", err)
	}
	if resA.Action != "fast-forward" {
		t.Fatalf("expected A to fast-forward, got %q", resA.Action)
	}

	// Both clones now hold both memories — convergent, conflict-free.
	want := []string{idA, idB}
	sort.Strings(want)
	if got := memoryIDs(t, lA); !equalStrings(got, want) {
		t.Fatalf("clone A memories = %v, want %v", got, want)
	}
	if got := memoryIDs(t, lB); !equalStrings(got, want) {
		t.Fatalf("clone B memories = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
