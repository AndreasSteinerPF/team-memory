package retrieve_test

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

const branch = "teammemory"

func newLedger(t *testing.T) (*ledger.Ledger, git.Runner, string) {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	mustGit(t, r, "init", "-q", "-b", "main")
	mustGit(t, r, "config", "user.email", "test@example.com")
	mustGit(t, r, "config", "user.name", "Test")
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	if err := l.Init(nil); err != nil {
		t.Fatalf("init ledger: %v", err)
	}
	return l, r, dir
}

func mustGit(t *testing.T, r git.Runner, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func openIndex(t *testing.T, src index.Source) *index.Index {
	t.Helper()
	idx, err := index.Open(filepath.Join(t.TempDir(), "index.db"), src)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func idsOf(rs []retrieve.Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Memory.ID
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestEndToEndScopeAndFTSRetrieval(t *testing.T) {
	l, _, _ := newLedger(t)

	// A specific active memory on the migrations path.
	migID, err := l.AppendMemory(model.Memory{
		Type:    model.TypeFailedAttempt,
		Title:   "billing migrations need downgrade tests",
		Summary: "rollback failed without a downgrade path",
		Scope:   model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append mig: %v", err)
	}
	// Activate it: low->medium tier needs one independent confirm.
	if _, err := l.AppendObservation(model.Observation{
		Target: migID, Kind: model.KindConfirm,
		Summary: "same rollback failure on another branch",
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "b", SessionID: "s2"},
	}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	// A broad active memory that also matches the migrations path (less specific).
	broadID, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "all billing changes need a changelog entry",
		Scope: model.Scope{Paths: []string{"billing/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append broad: %v", err)
	}

	// An FTS-only hit: matches the description text but not the path.
	ftsID, err := l.AppendMemory(model.Memory{
		Type:    model.TypeFragileArea,
		Title:   "webhook retries are fragile",
		Summary: "duplicate deliveries cause double charges",
		Scope:   model.Scope{Paths: []string{"notifications/**"}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append fts: %v", err)
	}
	// Activate it too (medium tier needs one independent confirm), so it is
	// retrievable as an FTS-only hit rather than filtered out as provisional.
	if _, err := l.AppendObservation(model.Observation{
		Target: ftsID, Kind: model.KindConfirm,
		Summary: "saw duplicate webhook deliveries again",
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "b", SessionID: "s2"},
	}); err != nil {
		t.Fatalf("confirm fts: %v", err)
	}

	idx := openIndex(t, l)
	e := retrieve.New(idx, nil, policy.Default())

	got, err := e.Retrieve(retrieve.Query{
		Paths:       []string{"billing/migrations/2026_add_invoice_state.sql"},
		Description: "webhook duplicate delivery",
	})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	gotIDs := idsOf(got)
	// Scope matches outrank the FTS-only hit; the more specific scope wins.
	if len(gotIDs) < 3 {
		t.Fatalf("got %v, want at least the 3 candidates", gotIDs)
	}
	if gotIDs[0] != migID {
		t.Fatalf("most specific match should rank first; got %v (mig=%s)", gotIDs, migID)
	}
	if !contains(gotIDs, broadID) || !contains(gotIDs, ftsID) {
		t.Fatalf("expected broad (%s) and fts (%s) in %v", broadID, ftsID, gotIDs)
	}
	// The FTS-only hit must rank below both scope matches.
	posFTS, posBroad := indexOf(gotIDs, ftsID), indexOf(gotIDs, broadID)
	if posFTS < posBroad {
		t.Fatalf("FTS-only hit %s ranked above scope match %s: %v", ftsID, broadID, gotIDs)
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func TestEndToEndAnchorDriftAnnotation(t *testing.T) {
	l, r, dir := newLedger(t)

	// Build real code history: commit the anchored file, capture the SHA, then
	// change it twice more so drift = 2 commits.
	if err := os.WriteFile(filepath.Join(dir, "billing_migration.sql"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, r, "add", "billing_migration.sql")
	mustGit(t, r, "commit", "-q", "-m", "add migration")
	anchorCommit := mustGit(t, r, "rev-parse", "HEAD")
	for _, v := range []string{"v2", "v3"} {
		if err := os.WriteFile(filepath.Join(dir, "billing_migration.sql"), []byte(v), 0o644); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
		mustGit(t, r, "commit", "-q", "-am", "change migration")
	}

	id, err := l.AppendMemory(model.Memory{
		Type:    model.TypeFailedAttempt,
		Title:   "migration needs downgrade test",
		Scope:   model.Scope{Paths: []string{"billing_migration.sql"}},
		Anchors: []model.Anchor{{Path: "billing_migration.sql", Commit: anchorCommit}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	idx := openIndex(t, l)
	e := retrieve.New(idx, retrieve.GitDrift{Git: r}, policy.Default())

	got, err := e.Retrieve(retrieve.Query{Paths: []string{"billing_migration.sql"}})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if len(got) != 1 || got[0].Memory.ID != id {
		t.Fatalf("got %v, want the single memory %s", idsOf(got), id)
	}
	if len(got[0].Drift) != 1 || got[0].Drift[0].CommitsChanged != 2 || got[0].Drift[0].Note == "" {
		t.Fatalf("expected drift of 2 commits with a note, got %+v", got[0].Drift)
	}
}
