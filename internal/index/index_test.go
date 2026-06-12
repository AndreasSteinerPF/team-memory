package index_test

import (
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

const branch = "teammemory"

// newLedger builds a fresh git repo with an initialised ledger (default policy).
func newLedger(t *testing.T) *ledger.Ledger {
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
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init ledger: %v", err)
	}
	return l
}

func mustGit(t *testing.T, r git.Runner, args ...string) {
	t.Helper()
	if _, err := r.Run(args...); err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
}

func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "index.db")
}

func openIndex(t *testing.T, dst string, src index.Source) *index.Index {
	t.Helper()
	idx, err := index.Open(dst, src)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func TestReindexMaterializesDerivedState(t *testing.T) {
	l := newLedger(t)
	mem := model.Memory{
		Type:     model.TypeFailedAttempt,
		Title:    "billing migrations need downgrade tests",
		Summary:  "downgrade path was untested",
		Guidance: "add a downgrade test before merging",
		Scope:    model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:    model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	}
	id, err := l.AppendMemory(mem)
	if err != nil {
		t.Fatalf("append memory: %v", err)
	}

	idx := openIndex(t, dbPath(t), l) // Open runs a full Reindex on a fresh db

	all, err := idx.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d rows, want 1", len(all))
	}
	got := all[0]

	// The materialized row must equal what derive.Derive computes directly.
	stored, _, _ := l.Memory(id)
	want := derive.Derive(stored, nil, policy.Default())
	if got.ID != id || got.Status != want.Status || got.Risk != want.Risk ||
		got.Confidence != want.Confidence || got.Enforcement != want.Enforcement {
		t.Fatalf("row %+v does not match derived state %+v", got, want)
	}
	if !reflect.DeepEqual(got.EffectiveScope, want.EffectiveScope.Paths) {
		t.Fatalf("scope = %v, want %v", got.EffectiveScope, want.EffectiveScope.Paths)
	}
	if got.Title != mem.Title {
		t.Fatalf("title = %q, want %q", got.Title, mem.Title)
	}
}

func TestReindexIsIdempotent(t *testing.T) {
	l := newLedger(t)
	for i := 0; i < 3; i++ {
		if _, err := l.AppendMemory(model.Memory{
			Type:  model.TypeDecision,
			Title: "decision",
			Scope: model.Scope{Paths: []string{"src/**"}},
			Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
		}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	idx := openIndex(t, dbPath(t), l)

	first, err := idx.All()
	if err != nil {
		t.Fatalf("all #1: %v", err)
	}
	if err := idx.Reindex(); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	second, err := idx.All()
	if err != nil {
		t.Fatalf("all #2: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("reindex not idempotent:\n#1=%+v\n#2=%+v", first, second)
	}
}

func TestSearchIDsFindsByText(t *testing.T) {
	l := newLedger(t)
	hit, err := l.AppendMemory(model.Memory{
		Type:    model.TypeFragileArea,
		Title:   "payment webhook retries are fragile",
		Summary: "duplicate deliveries cause double charges",
		Scope:   model.Scope{Paths: []string{"billing/**"}},
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append hit: %v", err)
	}
	if _, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "use UTC timestamps everywhere",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s2"},
	}); err != nil {
		t.Fatalf("append miss: %v", err)
	}

	idx := openIndex(t, dbPath(t), l)
	ids, err := idx.SearchIDs("webhook")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(ids) != 1 || ids[0] != hit {
		t.Fatalf("search ids = %v, want [%s]", ids, hit)
	}
}
