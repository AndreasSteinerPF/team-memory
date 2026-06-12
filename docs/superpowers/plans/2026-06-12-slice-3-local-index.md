# Slice 3 — Local Index Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Materialize the ledger's derived state into a local SQLite index (`.git/tm/index.db`) with an FTS table, rebuildable by full replay, updatable incrementally on sync, and self-healing on corruption or version mismatch.

**Architecture:** A new `internal/index` package owns a SQLite database (pure-Go `modernc.org/sqlite`, so Windows CI needs no C toolchain). It reads records through a small `Source` interface (satisfied by `*ledger.Ledger`), runs the pure `derive.Derive` function per memory, and writes one materialized row per memory plus a parallel FTS5 row over title/summary/guidance. `Reindex()` does a full replay (used by `tm init`/`tm reindex`); `Update()` re-derives only the memories affected by records added since the last indexed commit (used after sync). `Open()` validates the stored schema version and rebuilds from scratch on any problem — the ledger branch is always the source of truth. The defining correctness property is **`index == replay`**: any interleaving of incremental updates yields byte-for-byte the same materialized rows as a fresh full replay.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go SQLite driver + FTS5), `database/sql`, existing `internal/{model,policy,derive,ledger,git}` packages.

---

## File Structure

- `internal/ledger/ledger.go` (modify) — add read primitives the index needs: `Tip`, `Memory`, `ChangedSince`, `GitDir`.
- `internal/ledger/source_test.go` (create) — tests for the new ledger primitives.
- `internal/index/index.go` (create) — `Source` interface, `Index` type, `Open`/rebuild/`createSchema`/`Close`, `PathFor`, version constants.
- `internal/index/replay.go` (create) — `Reindex` (full replay), `Update` (incremental), and derivation/upsert helpers.
- `internal/index/query.go` (create) — `IndexedMemory` row type, `All`, `SearchIDs`.
- `internal/index/index_test.go` (create) — materialization, idempotency, FTS, incremental==replay, the property test, and auto-rebuild tests.
- `go.mod` / `go.sum` (modify) — add `modernc.org/sqlite`.

No CI change: `modernc.org/sqlite` is pure Go, so the existing `{ubuntu, macos, windows}` matrix (with `-race` on Unix) builds and runs as-is.

---

### Task 1: Ledger read primitives for the index

The index reads through a narrow interface. Add the four methods `*ledger.Ledger` must expose: the current branch tip, a single memory by ID, the record paths changed since a commit, and the absolute `.git` dir (for locating the index file in Slice 5).

**Files:**
- Modify: `internal/ledger/ledger.go`
- Create: `internal/ledger/source_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ledger/source_test.go`:

```go
package ledger_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func containsStr(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func TestTipIsEmptyBeforeInitAndSetAfter(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if tip, err := l.Tip(); err != nil || tip != "" {
		t.Fatalf("tip before init = %q, %v; want empty, nil", tip, err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	tip, err := l.Tip()
	if err != nil || tip == "" {
		t.Fatalf("tip after init = %q, %v; want non-empty", tip, err)
	}
}

func TestMemoryReadsSingleRecordAndReportsAbsent(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("x: y\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "hello",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	m, ok, err := l.Memory(id)
	if err != nil || !ok {
		t.Fatalf("Memory(%q) = ok %v, err %v; want ok true", id, ok, err)
	}
	if m.Title != "hello" {
		t.Fatalf("title = %q, want hello", m.Title)
	}

	if _, ok, err := l.Memory("01NOTAREALRECORDIDXXXXXXXX"); err != nil || ok {
		t.Fatalf("Memory(missing) = ok %v, err %v; want ok false, nil", ok, err)
	}
}

func TestChangedSinceListsAddedRecordPaths(t *testing.T) {
	dir := newRepo(t)
	l, err := ledger.Open(dir, branch)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := l.Init([]byte("retrieval:\n  max_results: 5\n")); err != nil {
		t.Fatalf("init: %v", err)
	}
	base, err := l.Tip()
	if err != nil {
		t.Fatalf("tip: %v", err)
	}

	id, err := l.AppendMemory(model.Memory{
		Type:  model.TypeDecision,
		Title: "x",
		Actor: model.Actor{Kind: model.ActorAgent, Name: "a", SessionID: "s"},
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	paths, cur, err := l.ChangedSince(base)
	if err != nil {
		t.Fatalf("changed since: %v", err)
	}
	if cur == base {
		t.Fatal("tip should have advanced after an append")
	}
	if want := "memories/" + id + ".yaml"; !containsStr(paths, want) {
		t.Fatalf("changed paths %v missing %q", paths, want)
	}

	// Nothing changed between the current tip and itself.
	paths2, _, err := l.ChangedSince(cur)
	if err != nil {
		t.Fatalf("changed since (current): %v", err)
	}
	if len(paths2) != 0 {
		t.Fatalf("expected no changes, got %v", paths2)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/ledger/ -run 'Tip|Memory|ChangedSince' -v`
Expected: compile failure — `l.Tip`, `l.Memory`, `l.ChangedSince` undefined.

- [ ] **Step 3: Add the methods**

In `internal/ledger/ledger.go`, append these methods (after `readFile`, before `tempIndex`):

```go
// Tip returns the current commit SHA of the ledger branch, or "" if the branch
// does not exist yet.
func (l *Ledger) Tip() (string, error) {
	if !l.Exists() {
		return "", nil
	}
	return l.git.Run("rev-parse", l.ref())
}

// Memory returns the memory with the given ID. The bool is false (with a nil
// error) if no such record exists on the branch.
func (l *Ledger) Memory(id string) (model.Memory, bool, error) {
	if !l.Exists() {
		return model.Memory{}, false, nil
	}
	data, err := l.readFile(memoriesDir + "/" + id + ".yaml")
	if err != nil {
		return model.Memory{}, false, nil // absent path ⇒ not found
	}
	m, err := unmarshalMemory(data)
	if err != nil {
		return model.Memory{}, false, fmt.Errorf("ledger: parse memory %s: %w", id, err)
	}
	return m, true, nil
}

// ChangedSince returns the record paths added or modified between commit old and
// the current branch tip, plus the current tip. When old is "", equals the
// current tip, or the branch is empty, no paths are returned. Records are
// append-only, so in practice only additions appear; modifications are still
// reported (e.g. an unexpected policy.yaml change) so callers can react.
func (l *Ledger) ChangedSince(old string) (paths []string, current string, err error) {
	current, err = l.Tip()
	if err != nil {
		return nil, "", err
	}
	if old == "" || current == "" || old == current {
		return nil, current, nil
	}
	out, err := l.git.Run("diff", "--name-only", "--diff-filter=AM", old, current)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(out) == "" {
		return nil, current, nil
	}
	return strings.Split(out, "\n"), current, nil
}

// GitDir returns the absolute path to the repository's .git directory. The local
// index and session-local state live under <GitDir>/tm/ (prd.md §7.3).
func (l *Ledger) GitDir() (string, error) {
	return l.git.Run("rev-parse", "--absolute-git-dir")
}
```

(`fmt` and `strings` are already imported by `ledger.go`.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ledger/ -run 'Tip|Memory|ChangedSince' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Full ledger suite still green**

Run: `go test ./internal/ledger/`
Expected: ok.

- [ ] **Step 6: Commit**

```bash
git add internal/ledger/ledger.go internal/ledger/source_test.go
git commit -m "feat(ledger): tip, single-memory, and changed-since read primitives for the index"
```

---

### Task 2: SQLite index — schema, open/rebuild, full replay, read & FTS

Add the SQLite dependency and the core index: open-or-rebuild semantics, schema creation (a materialized `memories` table + an FTS5 table + a `meta` table), full `Reindex()` replay, the `All()` materialized read, and a minimal `SearchIDs()` FTS query.

**Files:**
- Modify: `go.mod`, `go.sum`
- Create: `internal/index/index.go`, `internal/index/replay.go`, `internal/index/query.go`, `internal/index/index_test.go`

- [ ] **Step 1: Add the SQLite dependency**

Run:
```bash
go get modernc.org/sqlite@latest
go mod tidy
```
Expected: `modernc.org/sqlite` appears in `go.mod`'s require block; several `modernc.org/*` indirect deps are added. No code yet, so nothing builds against it.

- [ ] **Step 2: Write the failing test**

Create `internal/index/index_test.go`:

```go
package index_test

import (
	"database/sql"
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
	stored, _ := l.Memory(id)
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
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/index/ -run TestReindexMaterializesDerivedState -v`
Expected: compile failure — package `internal/index` has no Go files / `index.Open`, `index.Source`, `index.Index` undefined.

- [ ] **Step 4: Create `internal/index/index.go`**

```go
// Package index materializes the ledger's derived state into a local SQLite
// database (.git/tm/index.db) for fast retrieval. The database is a disposable
// cache: it is rebuilt from scratch by full replay (Reindex), updated
// incrementally after sync (Update), and automatically rebuilt on corruption or
// schema-version mismatch. The ledger branch is always the source of truth
// (prd.md §7.3).
package index

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// schemaVersion is bumped whenever the table layout or derivation semantics
// change in a way that invalidates an existing index. A stored value other than
// this triggers an automatic rebuild on Open.
const schemaVersion = "1"

const (
	metaSchemaVersion = "schema_version"
	metaLedgerTip     = "ledger_tip"
)

// Source is the subset of the ledger the index reads to materialize state.
// *ledger.Ledger satisfies it.
type Source interface {
	Tip() (string, error)
	Memory(id string) (model.Memory, bool, error)
	Memories() ([]model.Memory, error)
	Observations() ([]model.Observation, error)
	Policy() ([]byte, error)
	ChangedSince(old string) (paths []string, current string, err error)
}

// Index is a handle to the local SQLite index.
type Index struct {
	db  *sql.DB
	src Source
}

// PathFor returns the conventional index location inside a .git directory.
func PathFor(absoluteGitDir string) string {
	return filepath.Join(absoluteGitDir, "tm", "index.db")
}

// Open opens (or creates) the index at dbPath backed by src. If the database is
// missing, unreadable, or its schema version does not match, it is removed and
// rebuilt from the ledger by a full replay. A freshly created database is always
// replayed before returning.
func Open(dbPath string, src Source) (*Index, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("index: create dir: %w", err)
	}
	idx, fresh, err := openOrReset(dbPath, src)
	if err != nil {
		return nil, err
	}
	if fresh {
		if err := idx.Reindex(); err != nil {
			idx.Close()
			return nil, err
		}
	}
	return idx, nil
}

// openOrReset opens the db and validates its stored schema version. If the file
// is missing, unreadable, corrupt, or carries the wrong version, it is removed
// and recreated with an empty schema. The bool reports whether the caller must
// Reindex (true whenever a fresh, empty schema was created).
func openOrReset(dbPath string, src Source) (*Index, bool, error) {
	if db, err := openDB(dbPath); err == nil {
		var v string
		qerr := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaSchemaVersion).Scan(&v)
		if qerr == nil && v == schemaVersion {
			return &Index{db: db, src: src}, false, nil
		}
		db.Close() // missing/corrupt schema or wrong version ⇒ rebuild
	}
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("index: remove stale db: %w", err)
	}
	db, err := openDB(dbPath)
	if err != nil {
		return nil, false, err
	}
	idx := &Index{db: db, src: src}
	if err := idx.createSchema(); err != nil {
		idx.Close()
		return nil, false, err
	}
	return idx, true, nil
}

func openDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("index: open %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1) // single-writer local cache; avoids SQLITE_BUSY
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: ping %s: %w", dbPath, err)
	}
	return db, nil
}

func (idx *Index) createSchema() error {
	stmts := []string{
		`CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE memories (
  id                   TEXT PRIMARY KEY,
  type                 TEXT NOT NULL,
  origin               TEXT NOT NULL DEFAULT '',
  title                TEXT NOT NULL,
  summary              TEXT NOT NULL DEFAULT '',
  guidance             TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL,
  risk                 TEXT NOT NULL,
  confidence           TEXT NOT NULL,
  enforcement          TEXT NOT NULL,
  effective_scope      TEXT NOT NULL DEFAULT '[]',
  independent_confirms INTEGER NOT NULL DEFAULT 0,
  contradictions       INTEGER NOT NULL DEFAULT 0,
  reason               TEXT NOT NULL DEFAULT '',
  created_at           TEXT NOT NULL
)`,
		`CREATE VIRTUAL TABLE memories_fts USING fts5(id UNINDEXED, title, summary, guidance)`,
	}
	for _, s := range stmts {
		if _, err := idx.db.Exec(s); err != nil {
			return fmt.Errorf("index: create schema: %w", err)
		}
	}
	if _, err := idx.db.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)`,
		metaSchemaVersion, schemaVersion); err != nil {
		return fmt.Errorf("index: write schema version: %w", err)
	}
	return nil
}

// Close releases the database handle.
func (idx *Index) Close() error { return idx.db.Close() }
```

- [ ] **Step 5: Create `internal/index/replay.go`**

```go
package index

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// Reindex rebuilds every materialized row from a full ledger replay. It is
// idempotent: replaying the same ledger twice yields identical rows.
func (idx *Index) Reindex() error {
	mems, err := idx.src.Memories()
	if err != nil {
		return err
	}
	obs, err := idx.src.Observations()
	if err != nil {
		return err
	}
	pol, err := idx.loadPolicy()
	if err != nil {
		return err
	}
	tip, err := idx.src.Tip()
	if err != nil {
		return err
	}
	byTarget := groupByTarget(obs)

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM memories`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memories_fts`); err != nil {
		return err
	}
	for _, m := range mems {
		if err := upsertTx(tx, m, derive.Derive(m, byTarget[m.ID], pol)); err != nil {
			return err
		}
	}
	if err := setTipTx(tx, tip); err != nil {
		return err
	}
	return tx.Commit()
}

// Update brings the index up to the current ledger tip by re-deriving only the
// memories affected by records added since the last indexed commit. It is a
// no-op if the ledger has not advanced. A change to policy.yaml (which can alter
// every memory's state) forces a full Reindex.
func (idx *Index) Update() error {
	old, err := idx.storedTip()
	if err != nil {
		return err
	}
	if old == "" {
		return idx.Reindex() // never indexed ⇒ full replay
	}
	paths, current, err := idx.src.ChangedSince(old)
	if err != nil {
		return err
	}
	if current == old {
		return nil // up to date
	}
	for _, p := range paths {
		if p == "policy.yaml" {
			return idx.Reindex()
		}
	}

	obs, err := idx.src.Observations()
	if err != nil {
		return err
	}
	byID := make(map[string]model.Observation, len(obs))
	for _, o := range obs {
		byID[o.ID] = o
	}
	byTarget := groupByTarget(obs)
	affected := affectedMemoryIDs(paths, byID)

	pol, err := idx.loadPolicy()
	if err != nil {
		return err
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id := range affected {
		m, ok, err := idx.src.Memory(id)
		if err != nil {
			return err
		}
		if !ok {
			continue // observation referencing a not-yet-present memory
		}
		if err := upsertTx(tx, m, derive.Derive(m, byTarget[id], pol)); err != nil {
			return err
		}
	}
	if err := setTipTx(tx, current); err != nil {
		return err
	}
	return tx.Commit()
}

func (idx *Index) loadPolicy() (policy.Policy, error) {
	data, err := idx.src.Policy()
	if err != nil {
		return policy.Default(), nil // empty ledger ⇒ built-in defaults
	}
	return policy.Load(data)
}

func (idx *Index) storedTip() (string, error) {
	var tip string
	err := idx.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaLedgerTip).Scan(&tip)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return tip, err
}

func groupByTarget(obs []model.Observation) map[string][]model.Observation {
	m := make(map[string][]model.Observation)
	for _, o := range obs {
		m[o.Target] = append(m[o.Target], o)
	}
	return m
}

func affectedMemoryIDs(paths []string, byID map[string]model.Observation) map[string]struct{} {
	affected := make(map[string]struct{})
	for _, p := range paths {
		switch {
		case isRecordPath(p, "memories/"):
			affected[recordID(p, "memories/")] = struct{}{}
		case isRecordPath(p, "observations/"):
			if o, ok := byID[recordID(p, "observations/")]; ok {
				affected[o.Target] = struct{}{}
			}
		}
	}
	return affected
}

func isRecordPath(p, prefix string) bool {
	return len(p) > len(prefix)+5 && p[:len(prefix)] == prefix && p[len(p)-5:] == ".yaml"
}

func recordID(p, prefix string) string {
	return p[len(prefix) : len(p)-5]
}

func upsertTx(tx *sql.Tx, m model.Memory, st derive.DerivedState) error {
	paths := st.EffectiveScope.Paths
	if paths == nil {
		paths = []string{}
	}
	scopeJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
INSERT INTO memories (id, type, origin, title, summary, guidance, status, risk,
  confidence, enforcement, effective_scope, independent_confirms, contradictions,
  reason, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  type=excluded.type, origin=excluded.origin, title=excluded.title,
  summary=excluded.summary, guidance=excluded.guidance, status=excluded.status,
  risk=excluded.risk, confidence=excluded.confidence,
  enforcement=excluded.enforcement, effective_scope=excluded.effective_scope,
  independent_confirms=excluded.independent_confirms,
  contradictions=excluded.contradictions, reason=excluded.reason,
  created_at=excluded.created_at`,
		m.ID, string(m.Type), string(m.Origin), m.Title, m.Summary, m.Guidance,
		string(st.Status), string(st.Risk), string(st.Confidence), string(st.Enforcement),
		string(scopeJSON), st.IndependentConfirms, st.Contradictions, st.Reason,
		m.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM memories_fts WHERE id = ?`, m.ID); err != nil {
		return err
	}
	_, err = tx.Exec(
		`INSERT INTO memories_fts (id, title, summary, guidance) VALUES (?,?,?,?)`,
		m.ID, m.Title, m.Summary, m.Guidance)
	return err
}

func setTipTx(tx *sql.Tx, tip string) error {
	_, err := tx.Exec(`INSERT INTO meta(key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, metaLedgerTip, tip)
	return err
}
```

- [ ] **Step 6: Create `internal/index/query.go`**

```go
package index

import (
	"encoding/json"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// IndexedMemory is one materialized row: a memory's stored fields plus its
// derived state.
type IndexedMemory struct {
	ID                  string
	Type                model.MemoryType
	Origin              model.ConstraintOrigin
	Title               string
	Summary             string
	Guidance            string
	Status              model.Status
	Risk                model.Risk
	Confidence          model.Confidence
	Enforcement         model.Enforcement
	EffectiveScope      []string
	IndependentConfirms int
	Contradictions      int
	Reason              string
	CreatedAt           time.Time
}

// All returns every materialized memory ordered by ID (deterministic, for
// inspection and tests).
func (idx *Index) All() ([]IndexedMemory, error) {
	rows, err := idx.db.Query(`
SELECT id, type, origin, title, summary, guidance, status, risk, confidence,
  enforcement, effective_scope, independent_confirms, contradictions, reason, created_at
FROM memories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []IndexedMemory
	for rows.Next() {
		var im IndexedMemory
		var typ, origin, status, risk, conf, enf, scopeJSON, createdAt string
		if err := rows.Scan(&im.ID, &typ, &origin, &im.Title, &im.Summary, &im.Guidance,
			&status, &risk, &conf, &enf, &scopeJSON, &im.IndependentConfirms,
			&im.Contradictions, &im.Reason, &createdAt); err != nil {
			return nil, err
		}
		im.Type = model.MemoryType(typ)
		im.Origin = model.ConstraintOrigin(origin)
		im.Status = model.Status(status)
		im.Risk = model.Risk(risk)
		im.Confidence = model.Confidence(conf)
		im.Enforcement = model.Enforcement(enf)
		if err := json.Unmarshal([]byte(scopeJSON), &im.EffectiveScope); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		im.CreatedAt = t
		out = append(out, im)
	}
	return out, rows.Err()
}

// SearchIDs returns the IDs of memories whose title, summary, or guidance match
// the FTS query, most-relevant first. It validates the FTS table; full retrieval
// ranking lands in Slice 4.
func (idx *Index) SearchIDs(query string) ([]string, error) {
	rows, err := idx.db.Query(
		`SELECT id FROM memories_fts WHERE memories_fts MATCH ? ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/index/ -run 'Reindex|SearchIDs' -v`
Expected: PASS (`TestReindexMaterializesDerivedState`, `TestReindexIsIdempotent`, `TestSearchIDsFindsByText`).

- [ ] **Step 8: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/index/index.go internal/index/replay.go internal/index/query.go internal/index/index_test.go
git commit -m "feat(index): SQLite materialization of derived state with full replay and FTS"
```

---

### Task 3: Incremental update

Prove `Update()` re-derives exactly the memories touched by new records and converges to the same rows as a full replay, including the case where a late observation flips an existing memory's state.

**Files:**
- Modify: `internal/index/index_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/index/index_test.go`:

```go
func TestUpdateMatchesReplayAfterNewRecords(t *testing.T) {
	l := newLedger(t)

	// A medium-risk memory with a single same-session confirm: still provisional
	// (independence requires a *different* session).
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
```

- [ ] **Step 2: Run the tests to verify they pass**

Run: `go test ./internal/index/ -run 'TestUpdate' -v`
Expected: PASS. (`Update` and the supporting helpers already exist from Task 2; these tests assert the behavior. If `TestUpdateMatchesReplayAfterNewRecords` fails on the activation assertion, the bug is in the *test's* derivation expectation, not the index — re-derive with `derive.Derive` to confirm, but the engine's Slice-1 tests already pin `different_session` activation.)

- [ ] **Step 3: Commit**

```bash
git add internal/index/index_test.go
git commit -m "test(index): incremental update converges to full replay"
```

---

### Task 4: The `index == replay` property test

The defining invariant of the slice: for any random interleaving of memory/observation appends with incremental `Update()` calls, the index equals a fresh full replay of the same ledger.

**Files:**
- Modify: `internal/index/index_test.go`

- [ ] **Step 1: Write the property test**

Add to the import block of `internal/index/index_test.go`: `"fmt"` and `"math/rand"`. Then append:

```go
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
			const ops = 30 // kept modest: each op shells out to git; CI runs under -race
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
```

- [ ] **Step 2: Run the property test**

Run: `go test ./internal/index/ -run TestPropertyIndexEqualsReplay -v`
Expected: PASS (3 subtests). If a seed fails, the printed `inc`/`full` diff localizes the divergence; the bug is almost certainly in `affectedMemoryIDs` (a missed affected memory) or in not forcing a reindex when it should.

- [ ] **Step 3: Commit**

```bash
git add internal/index/index_test.go
git commit -m "test(index): property test asserting index == replay over random op sequences"
```

---

### Task 5: Auto-rebuild on corruption and version mismatch

`Open` already rebuilds when the schema version is wrong or the file is unreadable (Task 2's `openOrReset`). Pin that behavior with adversarial tests: a garbage file and a tampered schema version must both yield a correct, fully-replayed index.

**Files:**
- Modify: `internal/index/index_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/index/index_test.go`:

```go
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
	if v != "1" {
		t.Fatalf("schema_version = %q after rebuild, want \"1\"", v)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./internal/index/ -run TestAutoRebuild -v`
Expected: PASS (both). These exercise the `openOrReset` rebuild branch written in Task 2.

- [ ] **Step 3: Full slice suite, then build/vet/race**

Run:
```bash
go test ./internal/index/ ./internal/ledger/
go build ./... && go vet ./...
go test ./...
```
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add internal/index/index_test.go
git commit -m "test(index): auto-rebuild on corruption and schema-version mismatch"
```

---

### Task 6: Commit the plan document

**Files:**
- Create: `docs/superpowers/plans/2026-06-12-slice-3-local-index.md` (this file)

- [ ] **Step 1: Commit the plan**

```bash
git add docs/superpowers/plans/2026-06-12-slice-3-local-index.md
git commit -m "docs: slice-3 local index implementation plan"
```

---

## Self-Review

**Spec coverage (PRD §7.3, decomposition Slice 3 row):**
- "SQLite materialization of derived state" → `memories` table + `upsertTx(Derive(...))`; Task 2.
- "+ FTS" → `memories_fts` FTS5 table + `SearchIDs`; Task 2.
- "full replay (`reindex`)" → `Reindex()`; Task 2.
- "incremental update on sync" → `Update()`; Task 3.
- "auto-rebuild on corruption / version mismatch" → `openOrReset`; tested in Task 5.
- "`.git/tm/index.db`" → `PathFor` + `ledger.GitDir`; Task 1/2.
- "**`index == replay` invariant** (property test)" → Task 4.
- "idempotent replay" → `TestReindexIsIdempotent`; Task 2.

**Placeholder scan:** none — every step carries full code or an exact command.

**Type consistency:** `Source` methods match the signatures added to `*ledger.Ledger` in Task 1 (`Tip`, `Memory`, `Memories`, `Observations`, `Policy`, `ChangedSince`). `IndexedMemory.EffectiveScope` is `[]string` and is compared against `derive.DerivedState.EffectiveScope.Paths` (also `[]string`). `schemaVersion` constant (`"1"`) matches the literal asserted in Task 5.

**Determinism:** `Derive` already sorts observations by time then ID (Slice 1), so map-iteration order in `Update` and observation grouping cannot affect results. `created_at` is stored/parsed as RFC3339Nano UTC, so two indexes of the same ledger produce `reflect.DeepEqual` rows. The property test uses fixed PRNG seeds.

**Cross-OS:** `modernc.org/sqlite` is pure Go (no cgo), so the Windows CI leg builds without a C toolchain, matching the existing matrix.
