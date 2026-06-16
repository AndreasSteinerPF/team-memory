package index_test

import (
	"database/sql"
	"fmt"
	"math/rand"
	"os"
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

func TestUpdateMatchesReplayAfterNewRecords(t *testing.T) {
	l := newLedger(t)

	// A medium-risk memory with no confirms yet: still provisional.
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeFailedAttempt,
		Title: "retry storm on webhook",
		Scope: model.Scope{Paths: []string{"billing/webhook.go"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	})
	if err != nil {
		t.Fatalf("append memory: %v", err)
	}

	idx := openIndex(t, dbPath(t), l) // initial full replay

	before, err := idx.All()
	if err != nil {
		t.Fatalf("all before: %v", err)
	}
	if before[0].Status != model.StatusProvisional {
		t.Fatalf("status before confirm = %q, want provisional", before[0].Status)
	}

	// An independent confirm from a different session should activate it.
	if _, err := l.AppendObservation(model.Observation{
		Target:  id,
		Kind:    model.KindConfirm,
		Summary: "hit the same retry storm",
		Actor:   model.Actor{Kind: model.ActorAgent, Name: "b", SessionID: "s2"},
	}); err != nil {
		t.Fatalf("append observation: %v", err)
	}
	// A brand-new, unrelated memory added in the same window.
	if _, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "use UTC everywhere",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "c", SessionID: "s3"},
	}); err != nil {
		t.Fatalf("append decision: %v", err)
	}

	if err := idx.Update(); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The incrementally-updated index must equal a fresh full replay.
	full := openIndex(t, dbPath(t), l)
	gotInc, err := idx.All()
	if err != nil {
		t.Fatalf("all inc: %v", err)
	}
	gotFull, err := full.All()
	if err != nil {
		t.Fatalf("all full: %v", err)
	}
	if !reflect.DeepEqual(gotInc, gotFull) {
		t.Fatalf("incremental != replay:\n inc=%+v\nfull=%+v", gotInc, gotFull)
	}

	// And the confirm must actually have changed the affected memory's state.
	var updated index.IndexedMemory
	for _, m := range gotInc {
		if m.ID == id {
			updated = m
		}
	}
	if updated.Status == model.StatusProvisional {
		t.Fatalf("memory %s still provisional after independent confirm", id)
	}
}

func TestUpdateIsNoOpWhenLedgerUnchanged(t *testing.T) {
	l := newLedger(t)
	if _, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "x",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	idx := openIndex(t, dbPath(t), l)

	before, err := idx.All()
	if err != nil {
		t.Fatalf("all before: %v", err)
	}
	if err := idx.Update(); err != nil { // no new records
		t.Fatalf("update: %v", err)
	}
	after, err := idx.All()
	if err != nil {
		t.Fatalf("all after: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("no-op update changed rows:\nbefore=%+v\nafter=%+v", before, after)
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

func randomMemory(rng *rand.Rand) model.Memory {
	types := []model.MemoryType{
		model.TypeFailedAttempt, model.TypeConstraint, model.TypeFragileArea,
		model.TypeStaleDoc, model.TypeDecision,
	}
	scopes := [][]string{
		{"billing/migrations/**"}, {"auth/login.go"}, {"src/**"},
		{"docs/readme.md"}, {".github/workflows/ci.yml"},
	}
	m := model.Memory{
		Type:     types[rng.Intn(len(types))],
		Title:    fmt.Sprintf("memory about widget %d", rng.Intn(10000)),
		Summary:  "summary of an observed situation",
		Guidance: "do the thing carefully and test it",
		Scope:    model.Scope{Paths: scopes[rng.Intn(len(scopes))]},
		Actor: model.Actor{
			Kind: model.ActorAgent, Name: "agent",
			SessionID: fmt.Sprintf("s%d", rng.Intn(100)),
		},
	}
	if m.Type == model.TypeConstraint && rng.Intn(2) == 0 {
		m.Origin = model.OriginExternal
	}
	return m
}

func randomObservation(rng *rand.Rand, target string) model.Observation {
	kinds := []model.ObservationKind{
		model.KindConfirm, model.KindContradict, model.KindAdjustScope,
		model.KindMarkStale, model.KindApprove, model.KindReject,
	}
	k := kinds[rng.Intn(len(kinds))]
	o := model.Observation{
		Target:  target,
		Kind:    k,
		Summary: "observed something relevant",
		Actor: model.Actor{
			Kind: model.ActorAgent, Name: "observer",
			SessionID: fmt.Sprintf("o%d", rng.Intn(100)),
		},
	}
	switch k {
	case model.KindApprove:
		o.Actor.Kind = model.ActorHuman
		o.SetEnforcement = model.EnforcementWarning
	case model.KindReject:
		o.Actor.Kind = model.ActorHuman
	case model.KindAdjustScope:
		o.SuggestedScope = &model.Scope{Paths: []string{"newscope/**"}}
	}
	return o
}

func TestPropertyIndexEqualsReplay(t *testing.T) {
	for _, seed := range []int64{1, 42, 99} {
		seed := seed
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			l := newLedger(t)
			rng := rand.New(rand.NewSource(seed))

			inc := openIndex(t, filepath.Join(t.TempDir(), "inc.db"), l)

			var memIDs []string
			const ops = 30
			for i := 0; i < ops; i++ {
				if len(memIDs) == 0 || rng.Intn(3) == 0 {
					id, err := l.AppendMemory(randomMemory(rng))
					if err != nil {
						t.Fatalf("append memory: %v", err)
					}
					memIDs = append(memIDs, id)
				} else {
					target := memIDs[rng.Intn(len(memIDs))]
					if _, err := l.AppendObservation(randomObservation(rng, target)); err != nil {
						t.Fatalf("append observation: %v", err)
					}
				}
				if rng.Intn(4) == 0 { // sometimes sync the index mid-stream
					if err := inc.Update(); err != nil {
						t.Fatalf("update: %v", err)
					}
				}
			}
			if err := inc.Update(); err != nil {
				t.Fatalf("final update: %v", err)
			}

			full := openIndex(t, filepath.Join(t.TempDir(), "full.db"), l) // full replay

			gotInc, err := inc.All()
			if err != nil {
				t.Fatalf("all inc: %v", err)
			}
			gotFull, err := full.All()
			if err != nil {
				t.Fatalf("all full: %v", err)
			}
			if !reflect.DeepEqual(gotInc, gotFull) {
				t.Fatalf("index != replay (seed %d):\n inc=%+v\nfull=%+v", seed, gotInc, gotFull)
			}
		})
	}
}

func TestAutoRebuildOnCorruptFile(t *testing.T) {
	l := newLedger(t)
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "keep me",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	path := dbPath(t)

	first, err := index.Open(path, l)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	first.Close()

	// Corrupt the database file.
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	rebuilt, err := index.Open(path, l) // must detect corruption and rebuild
	if err != nil {
		t.Fatalf("open #2 (rebuild): %v", err)
	}
	defer rebuilt.Close()

	all, err := rebuilt.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 1 || all[0].ID != id {
		t.Fatalf("rebuilt index = %+v, want one row with id %s", all, id)
	}
}

func TestReindexStoresAnchors(t *testing.T) {
	l := newLedger(t)
	mem := model.Memory{
		Type:  model.TypeFailedAttempt,
		Title: "billing migrations need downgrade tests",
		Scope: model.Scope{Paths: []string{"billing/migrations/**"}},
		Anchors: []model.Anchor{
			{Path: "billing/migrations/2026_add_invoice_state.sql", Commit: "abc123"},
		},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s1"},
	}
	id, err := l.AppendMemory(mem)
	if err != nil {
		t.Fatalf("append memory: %v", err)
	}

	idx := openIndex(t, dbPath(t), l)
	all, err := idx.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 1 || all[0].ID != id {
		t.Fatalf("got %d rows, want 1 with id %s", len(all), id)
	}
	if !reflect.DeepEqual(all[0].Anchors, mem.Anchors) {
		t.Fatalf("anchors = %+v, want %+v", all[0].Anchors, mem.Anchors)
	}
}

func TestIndexStoresEffectiveCommands(t *testing.T) {
	l := newLedger(t)
	m := model.Memory{
		Type:  model.TypeConstraint,
		Title: "assistant jira create needs project",
		Scope: model.Scope{Commands: []string{"assistant jira create *"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "x", SessionID: "s1"},
	}
	id, err := l.AppendMemory(m)
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	idx := openIndex(t, dbPath(t), l)
	rows, err := idx.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].ID != id {
		t.Fatalf("ID = %s, want %s", rows[0].ID, id)
	}
	got := rows[0].EffectiveCommands
	if len(got) != 1 || got[0] != "assistant jira create *" {
		t.Fatalf("EffectiveCommands = %v, want [assistant jira create *]", got)
	}
}

func TestStatusByID(t *testing.T) {
	l := newLedger(t)
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "decision row",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	idx := openIndex(t, dbPath(t), l)

	// Happy path: existing row returns its materialized status.
	st, ok, err := idx.Status(id)
	if err != nil {
		t.Fatalf("status(existing): %v", err)
	}
	if !ok {
		t.Fatalf("status(existing) ok = false, want true")
	}
	if st == "" {
		t.Fatalf("status(existing) returned empty status")
	}
	// Must agree with the full materialized row.
	all, err := idx.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 1 || all[0].Status != st {
		t.Fatalf("Status(id)=%q, All()[0].Status=%q", st, all[0].Status)
	}

	// Not-found path: returns ("", false, nil).
	st2, ok, err := idx.Status("does-not-exist")
	if err != nil {
		t.Fatalf("status(missing): unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("status(missing) ok = true, want false")
	}
	if st2 != "" {
		t.Fatalf("status(missing) = %q, want empty", st2)
	}
}

func TestAutoRebuildOnSchemaVersionMismatch(t *testing.T) {
	l := newLedger(t)
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "keep me too",
		Scope: model.Scope{Paths: []string{"src/**"}},
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	path := dbPath(t)

	first, err := index.Open(path, l)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	first.Close()

	// Tamper with the stored schema version.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := db.Exec(`UPDATE meta SET value = '999' WHERE key = 'schema_version'`); err != nil {
		db.Close()
		t.Fatalf("tamper: %v", err)
	}
	db.Close()

	rebuilt, err := index.Open(path, l) // version mismatch ⇒ rebuild
	if err != nil {
		t.Fatalf("open #2 (rebuild): %v", err)
	}
	defer rebuilt.Close()

	all, err := rebuilt.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(all) != 1 || all[0].ID != id {
		t.Fatalf("rebuilt index = %+v, want one row with id %s", all, id)
	}

	// The rebuilt index must carry the current schema version again.
	verify, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("verify open: %v", err)
	}
	defer verify.Close()
	var v string
	if err := verify.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v); err != nil {
		t.Fatalf("read version: %v", err)
	}
	if v != "3" {
		t.Fatalf("schema_version = %q after rebuild, want \"3\"", v)
	}
}
