# Slice 4 — Retrieval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the precision-first, lexical retrieval engine (`internal/retrieve`) that turns an action (paths + description) into a ranked, capped set of memories with anchor-drift annotations and provisional caution framing — per prd.md §11 and §8.6.

**Architecture:** A new `internal/retrieve` package sits on top of the Slice 3 `internal/index`. It reads all materialized memories and FTS matches from the index, selects candidates (scope-glob match OR FTS match), filters by status, annotates anchor drift by shelling out to git in the code repo, ranks (glob specificity > status > enforcement > confidence > recency > anchor freshness), and applies output caps (`max_results` total, `max_provisional` for provisional/contested). Drift needs each memory's anchors, which the Slice 3 index does not store — so Task 1 extends the index schema (bumping it to v2; the existing auto-rebuild handles old indexes). The engine reads memories through a small `Index` interface and computes drift through a `DriftSource` interface, so ranking/filtering unit-test against fakes while integration tests use a real index and real git history.

**Tech Stack:** Go 1.26; `modernc.org/sqlite` (already a dep, FTS5); system `git` via `internal/git`; reuses `internal/derive`, `internal/model`, `internal/policy`, `internal/index`, `internal/ledger`.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/index/index.go` (modify) | Bump `schemaVersion` to `"2"`; add `anchors` column to the `memories` table. |
| `internal/index/replay.go` (modify) | `upsertTx` writes each memory's anchors as JSON. |
| `internal/index/query.go` (modify) | `IndexedMemory` gains `Anchors []model.Anchor`; `All` selects/parses it. |
| `internal/index/index_test.go` (modify) | New `TestReindexStoresAnchors`; update the schema-version assertion to `"2"`. |
| `internal/derive/scope.go` (modify) | Export `MatchPathGlob` / `MatchPath` (thin wrappers over the tested internals) for the retrieval layer. |
| `internal/derive/match_export_test.go` (create) | Verifies the exported wrappers. |
| `internal/retrieve/match.go` (create) | Glob specificity scoring, best-match selection over an action's paths, and FTS query sanitization. |
| `internal/retrieve/match_test.go` (create) | Unit tests for specificity ordering and FTS sanitization. |
| `internal/retrieve/retrieve.go` (create) | The engine: types, candidate assembly, status filtering, drift annotation, ranking, caps, provisional framing. |
| `internal/retrieve/retrieve_test.go` (create) | Unit tests with a fake index + fake drift source. |
| `internal/retrieve/drift.go` (create) | `GitDrift`: the real git-backed `DriftSource`. |
| `internal/retrieve/drift_test.go` (create) | Anchor-drift annotation against a real git history (temp repo). |
| `internal/retrieve/integration_test.go` (create) | End-to-end: real ledger → real index → `Engine.Retrieve` scope+FTS selection and ranking. |

---

## Task 1: Store anchors in the index (schema v2)

Drift annotation needs each memory's `(path, commit)` anchors. The Slice 3 index stores everything else but not anchors. Add them; bump the schema version so existing indexes auto-rebuild.

**Files:**
- Modify: `internal/index/index.go`
- Modify: `internal/index/replay.go`
- Modify: `internal/index/query.go`
- Test: `internal/index/index_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/index/index_test.go` (it already imports `model`, `reflect`, `testing`, and has `newLedger`, `dbPath`, `openIndex`):

```go
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/index/ -run TestReindexStoresAnchors`
Expected: FAIL — `IndexedMemory` has no field `Anchors` (compile error).

- [ ] **Step 3: Bump the schema version**

In `internal/index/index.go`, change:

```go
const schemaVersion = "1"
```

to:

```go
const schemaVersion = "2" // v2 adds the anchors column (Slice 4 drift)
```

- [ ] **Step 4: Add the anchors column to the schema**

In `internal/index/index.go`, in `createSchema`, replace the `memories` table statement's final column line. Change:

```go
  reason               TEXT NOT NULL DEFAULT '',
  created_at           TEXT NOT NULL
)`,
```

to:

```go
  reason               TEXT NOT NULL DEFAULT '',
  created_at           TEXT NOT NULL,
  anchors              TEXT NOT NULL DEFAULT '[]'
)`,
```

- [ ] **Step 5: Write anchors in `upsertTx`**

In `internal/index/replay.go`, `upsertTx` currently marshals only the effective scope. Add anchors marshaling at the top of the function, right after the existing `scopeJSON` block. Replace:

```go
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
```

with:

```go
	scopeJSON, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	anchors := m.Anchors
	if anchors == nil {
		anchors = []model.Anchor{}
	}
	anchorsJSON, err := json.Marshal(anchors)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`
INSERT INTO memories (id, type, origin, title, summary, guidance, status, risk,
  confidence, enforcement, effective_scope, independent_confirms, contradictions,
  reason, created_at, anchors)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  type=excluded.type, origin=excluded.origin, title=excluded.title,
  summary=excluded.summary, guidance=excluded.guidance, status=excluded.status,
  risk=excluded.risk, confidence=excluded.confidence,
  enforcement=excluded.enforcement, effective_scope=excluded.effective_scope,
  independent_confirms=excluded.independent_confirms,
  contradictions=excluded.contradictions, reason=excluded.reason,
  created_at=excluded.created_at, anchors=excluded.anchors`,
		m.ID, string(m.Type), string(m.Origin), m.Title, m.Summary, m.Guidance,
		string(st.Status), string(st.Risk), string(st.Confidence), string(st.Enforcement),
		string(scopeJSON), st.IndependentConfirms, st.Contradictions, st.Reason,
		m.CreatedAt.UTC().Format(time.RFC3339Nano), string(anchorsJSON),
	); err != nil {
		return err
	}
```

`replay.go` already imports `encoding/json` and `model`, so no new imports are needed.

- [ ] **Step 6: Expose anchors on `IndexedMemory`**

In `internal/index/query.go`, add the field to the struct. Change:

```go
	EffectiveScope      []string
	IndependentConfirms int
	Contradictions      int
	Reason              string
	CreatedAt           time.Time
}
```

to:

```go
	EffectiveScope      []string
	IndependentConfirms int
	Contradictions      int
	Reason              string
	CreatedAt           time.Time
	Anchors             []model.Anchor
}
```

- [ ] **Step 7: Select and parse anchors in `All`**

In `internal/index/query.go`, update the `All` query and scan. Change the SQL:

```go
SELECT id, type, origin, title, summary, guidance, status, risk, confidence,
  enforcement, effective_scope, independent_confirms, contradictions, reason, created_at
FROM memories ORDER BY id`)
```

to:

```go
SELECT id, type, origin, title, summary, guidance, status, risk, confidence,
  enforcement, effective_scope, independent_confirms, contradictions, reason,
  created_at, anchors
FROM memories ORDER BY id`)
```

Then change the scan target declaration:

```go
		var typ, origin, status, risk, conf, enf, scopeJSON, createdAt string
		if err := rows.Scan(&im.ID, &typ, &origin, &im.Title, &im.Summary, &im.Guidance,
			&status, &risk, &conf, &enf, &scopeJSON, &im.IndependentConfirms,
			&im.Contradictions, &im.Reason, &createdAt); err != nil {
			return nil, err
		}
```

to:

```go
		var typ, origin, status, risk, conf, enf, scopeJSON, createdAt, anchorsJSON string
		if err := rows.Scan(&im.ID, &typ, &origin, &im.Title, &im.Summary, &im.Guidance,
			&status, &risk, &conf, &enf, &scopeJSON, &im.IndependentConfirms,
			&im.Contradictions, &im.Reason, &createdAt, &anchorsJSON); err != nil {
			return nil, err
		}
```

And, right after the existing `EffectiveScope` unmarshal block, add anchor parsing. After:

```go
		if err := json.Unmarshal([]byte(scopeJSON), &im.EffectiveScope); err != nil {
			return nil, err
		}
```

add:

```go
		if err := json.Unmarshal([]byte(anchorsJSON), &im.Anchors); err != nil {
			return nil, err
		}
```

`query.go` already imports `encoding/json`, `time`, and `model`.

- [ ] **Step 8: Update the schema-version-mismatch test assertion**

The schema bump changes what a rebuilt index reports. In `internal/index/index_test.go`, in `TestAutoRebuildOnSchemaVersionMismatch`, change:

```go
	if v != "1" {
		t.Fatalf("schema_version = %q after rebuild, want \"1\"", v)
	}
```

to:

```go
	if v != "2" {
		t.Fatalf("schema_version = %q after rebuild, want \"2\"", v)
	}
```

- [ ] **Step 9: Run the index test suite**

Run: `go test ./internal/index/`
Expected: PASS (all existing tests plus `TestReindexStoresAnchors`).

- [ ] **Step 10: Commit**

```bash
git add internal/index/
git commit -m "feat(index): store memory anchors (schema v2) for drift annotation"
```

---

## Task 2: Glob matching + specificity + FTS sanitization

Retrieval matches an action's concrete paths against memories' effective-scope globs (reusing `derive`'s tested glob semantics), scores how specific each match is, and turns a free-text description into a safe FTS5 query.

**Files:**
- Modify: `internal/derive/scope.go`
- Test: `internal/derive/match_export_test.go` (create)
- Create: `internal/retrieve/match.go`
- Test: `internal/retrieve/match_test.go` (create)

- [ ] **Step 1: Write the failing test for the exported matchers**

Create `internal/derive/match_export_test.go`:

```go
package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestMatchPathGlob(t *testing.T) {
	cases := []struct {
		path, glob string
		want       bool
	}{
		{"billing/migrations/2026.sql", "billing/migrations/**", true},
		{"billing/migrations/2026.sql", "billing/*.go", false},
		{"anything/at/all", "**", true},
		{"src/main.go", "src/main.go", true},
	}
	for _, c := range cases {
		if got := MatchPathGlob(c.path, c.glob); got != c.want {
			t.Errorf("MatchPathGlob(%q,%q)=%v want %v", c.path, c.glob, got, c.want)
		}
	}
}

func TestMatchPath(t *testing.T) {
	s := model.Scope{Paths: []string{"auth/**", "src/main.go"}}
	if !MatchPath("auth/login.go", s) {
		t.Error("expected auth/login.go to match scope")
	}
	if MatchPath("docs/readme.md", s) {
		t.Error("did not expect docs/readme.md to match scope")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/derive/ -run 'TestMatchPath'`
Expected: FAIL — `MatchPathGlob`/`MatchPath` undefined.

- [ ] **Step 3: Export the matchers**

Append to `internal/derive/scope.go` (it already imports `model`):

```go
// MatchPathGlob reports whether a concrete path matches a single glob, using
// TeamMemory's segment-exact glob semantics. Exported for the retrieval layer.
func MatchPathGlob(path, glob string) bool { return pathMatchesGlob(path, glob) }

// MatchPath reports whether a concrete path matches any glob in scope.
func MatchPath(path string, s model.Scope) bool { return pathMatchesScope(path, s) }
```

- [ ] **Step 4: Run the derive test to verify it passes**

Run: `go test ./internal/derive/`
Expected: PASS.

- [ ] **Step 5: Write the failing test for match.go**

Create `internal/retrieve/match_test.go`:

```go
package retrieve

import "testing"

func TestGlobSpecificityOrdering(t *testing.T) {
	// More literal segments ⇒ more specific. The catch-all is least specific,
	// but any scope match still beats an FTS-only match (specificity 0).
	pairs := []struct{ more, less string }{
		{"billing/migrations/**", "billing/**"},
		{"billing/**", "**"},
		{"src/main.go", "src/**"},
	}
	for _, p := range pairs {
		if globSpecificity(p.more) <= globSpecificity(p.less) {
			t.Errorf("specificity(%q)=%d not > specificity(%q)=%d",
				p.more, globSpecificity(p.more), p.less, globSpecificity(p.less))
		}
	}
	if globSpecificity("**") < 1 {
		t.Errorf("a scope match must score >= 1 (got %d) so it beats FTS-only (0)",
			globSpecificity("**"))
	}
}

func TestBestSpecificity(t *testing.T) {
	scope := []string{"billing/**", "billing/migrations/**"}
	paths := []string{"billing/migrations/2026.sql"}
	score, matched := bestSpecificity(scope, paths)
	if !matched {
		t.Fatal("expected a match")
	}
	// Must pick the more specific of the two matching globs.
	if score != globSpecificity("billing/migrations/**") {
		t.Errorf("best specificity = %d, want %d", score, globSpecificity("billing/migrations/**"))
	}

	if _, m := bestSpecificity([]string{"auth/**"}, []string{"docs/x.md"}); m {
		t.Error("did not expect a match for non-overlapping scope")
	}
	if _, m := bestSpecificity([]string{"auth/**"}, nil); m {
		t.Error("no paths ⇒ no scope match")
	}
}

func TestFTSQuery(t *testing.T) {
	// Punctuation and FTS operators must be neutralized; tokens OR-joined.
	got := ftsQuery("rollback failure: invoice-state migration!")
	want := `"rollback" OR "failure" OR "invoice" OR "state" OR "migration"`
	if got != want {
		t.Errorf("ftsQuery = %q, want %q", got, want)
	}
	if ftsQuery("   ...  ") != "" {
		t.Errorf("punctuation-only description must yield empty query")
	}
	if ftsQuery("") != "" {
		t.Errorf("empty description must yield empty query")
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/retrieve/`
Expected: FAIL — package/functions do not exist yet.

- [ ] **Step 7: Implement match.go**

Create `internal/retrieve/match.go`:

```go
package retrieve

import (
	"strings"
	"unicode"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
)

// segments splits a glob/path into path segments, trimming slashes.
func segments(s string) []string {
	s = strings.Trim(s, "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func hasWildcard(seg string) bool { return strings.ContainsAny(seg, "*?[") }

// globSpecificity scores how precise a glob is. Any scope match scores >= 1 so
// it outranks an FTS-only match (specificity 0); each literal (non-wildcard)
// segment adds 2; wildcard segments and the catch-all "**" add nothing.
func globSpecificity(glob string) int {
	score := 1
	for _, seg := range segments(glob) {
		if seg != "**" && !hasWildcard(seg) {
			score += 2
		}
	}
	return score
}

// bestSpecificity returns the highest specificity among scope globs that match
// any of the action's paths, and whether any matched at all.
func bestSpecificity(scope, paths []string) (int, bool) {
	best, matched := 0, false
	for _, glob := range scope {
		for _, p := range paths {
			if derive.MatchPathGlob(p, glob) {
				if spec := globSpecificity(glob); !matched || spec > best {
					best, matched = spec, true
				}
				break
			}
		}
	}
	return best, matched
}

// ftsQuery turns a free-text description into a safe FTS5 MATCH expression:
// alphanumeric tokens, each quoted (neutralizing FTS operators), OR-joined for
// recall. Returns "" when there is nothing to search.
func ftsQuery(desc string) string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range desc {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(tokens) == 0 {
		return ""
	}
	for i, t := range tokens {
		tokens[i] = `"` + t + `"`
	}
	return strings.Join(tokens, " OR ")
}
```

- [ ] **Step 8: Run the retrieve tests to verify they pass**

Run: `go test ./internal/retrieve/`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/derive/scope.go internal/derive/match_export_test.go internal/retrieve/match.go internal/retrieve/match_test.go
git commit -m "feat(retrieve): glob specificity, path matching, FTS query sanitization"
```

---

## Task 3: The retrieval engine — candidates, filtering, drift hook, ranking, caps

The core `Engine.Retrieve`. This task delivers the complete engine, including the drift-annotation step gated on an injected `DriftSource` (the real git adapter arrives in Task 4; tests here use a fake). `DriftSource`, `DriftInfo`, and the note builders live here with the engine; only the git-backed adapter is separate.

**Files:**
- Create: `internal/retrieve/retrieve.go`
- Test: `internal/retrieve/retrieve_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/retrieve/retrieve_test.go`:

```go
package retrieve

import (
	"reflect"
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// fakeIndex serves canned rows; SearchIDs returns ids whose text was registered.
type fakeIndex struct {
	rows []index.IndexedMemory
	fts  map[string][]string // query → ordered ids
}

func (f *fakeIndex) All() ([]index.IndexedMemory, error) { return f.rows, nil }
func (f *fakeIndex) SearchIDs(q string) ([]string, error) {
	return f.fts[q], nil
}

// fakeDrift reports canned drift keyed by path.
type fakeDrift struct {
	missing map[string]bool
	changed map[string]int
}

func (f fakeDrift) Drift(path, _ string) (bool, int, error) {
	if f.missing[path] {
		return false, 0, nil
	}
	return true, f.changed[path], nil
}

func mem(id string, status model.Status, scope []string) index.IndexedMemory {
	return index.IndexedMemory{
		ID:             id,
		Type:           model.TypeFailedAttempt,
		Title:          id,
		Status:         status,
		Risk:           model.RiskMedium,
		Confidence:     model.ConfidenceLow,
		Enforcement:    model.EnforcementWarning,
		EffectiveScope: scope,
		CreatedAt:      time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
}

func ids(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Memory.ID
	}
	return out
}

func TestScopeAndFTSCandidateSelection(t *testing.T) {
	fi := &fakeIndex{
		rows: []index.IndexedMemory{
			mem("scoped", model.StatusActive, []string{"billing/**"}),
			mem("ftsonly", model.StatusActive, []string{"unrelated/**"}),
			mem("neither", model.StatusActive, []string{"nope/**"}),
		},
		fts: map[string][]string{`"webhook"`: {"ftsonly"}},
	}
	e := New(fi, nil, policy.Default())
	got, err := e.Retrieve(Query{Paths: []string{"billing/x.go"}, Description: "webhook"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	// "neither" must be excluded; scoped (specificity>0) ranks above fts-only.
	if !reflect.DeepEqual(ids(got), []string{"scoped", "ftsonly"}) {
		t.Fatalf("candidates = %v, want [scoped ftsonly]", ids(got))
	}
}

func TestStaleAndRejectedExcluded(t *testing.T) {
	fi := &fakeIndex{rows: []index.IndexedMemory{
		mem("stale", model.StatusStale, []string{"billing/**"}),
		mem("rejected", model.StatusRejected, []string{"billing/**"}),
		mem("ok", model.StatusActive, []string{"billing/**"}),
	}}
	e := New(fi, nil, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	if !reflect.DeepEqual(ids(got), []string{"ok"}) {
		t.Fatalf("got %v, want [ok]", ids(got))
	}
}

func TestProvisionalOnlyOnScopeMatchAndCautionFramed(t *testing.T) {
	fi := &fakeIndex{
		rows: []index.IndexedMemory{
			mem("prov_scope", model.StatusProvisional, []string{"billing/**"}),
			mem("prov_fts", model.StatusProvisional, []string{"unrelated/**"}),
		},
		fts: map[string][]string{`"webhook"`: {"prov_fts"}},
	}
	e := New(fi, nil, policy.Default()) // default provisional_mode = "related"
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}, Description: "webhook"})
	// related ⇒ provisional appears only on scope match, not FTS-only.
	if !reflect.DeepEqual(ids(got), []string{"prov_scope"}) {
		t.Fatalf("got %v, want [prov_scope]", ids(got))
	}
	if !got[0].Provisional || got[0].Caution == "" || got[0].Request == "" {
		t.Fatalf("provisional result must be caution-framed with a request: %+v", got[0])
	}
}

func TestProvisionalModeNever(t *testing.T) {
	fi := &fakeIndex{rows: []index.IndexedMemory{
		mem("prov", model.StatusProvisional, []string{"billing/**"}),
	}}
	e := New(fi, nil, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}, ProvisionalMode: "never"})
	if len(got) != 0 {
		t.Fatalf("provisional_mode=never must drop provisional, got %v", ids(got))
	}
}

func TestRankingBySpecificityThenEnforcement(t *testing.T) {
	broad := mem("broad", model.StatusActive, []string{"billing/**"})
	broad.Enforcement = model.EnforcementRequirement
	specific := mem("specific", model.StatusActive, []string{"billing/migrations/**"})
	specific.Enforcement = model.EnforcementHint
	fi := &fakeIndex{rows: []index.IndexedMemory{broad, specific}}
	e := New(fi, nil, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/migrations/2026.sql"}})
	// Specificity dominates enforcement (prd.md §11).
	if !reflect.DeepEqual(ids(got), []string{"specific", "broad"}) {
		t.Fatalf("ranking = %v, want [specific broad]", ids(got))
	}
}

func TestDriftRanksLowerAndAnnotates(t *testing.T) {
	fresh := mem("fresh", model.StatusActive, []string{"billing/**"})
	fresh.Anchors = []model.Anchor{{Path: "billing/fresh.go", Commit: "c0"}}
	drifted := mem("drifted", model.StatusActive, []string{"billing/**"})
	drifted.Anchors = []model.Anchor{{Path: "billing/drifted.go", Commit: "c0"}}
	fi := &fakeIndex{rows: []index.IndexedMemory{drifted, fresh}}
	fd := fakeDrift{changed: map[string]int{"billing/drifted.go": 9, "billing/fresh.go": 0}}
	e := New(fi, fd, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	// Same specificity/status/etc.; fresher anchor ranks first.
	if !reflect.DeepEqual(ids(got), []string{"fresh", "drifted"}) {
		t.Fatalf("ranking = %v, want [fresh drifted]", ids(got))
	}
	var d Result
	for _, r := range got {
		if r.Memory.ID == "drifted" {
			d = r
		}
	}
	if len(d.Drift) != 1 || d.Drift[0].CommitsChanged != 9 || d.Drift[0].Note == "" {
		t.Fatalf("drift annotation missing/wrong: %+v", d.Drift)
	}
}

func TestDriftMissingFile(t *testing.T) {
	m := mem("gone", model.StatusActive, []string{"billing/**"})
	m.Anchors = []model.Anchor{{Path: "billing/gone.go", Commit: "c0"}}
	fi := &fakeIndex{rows: []index.IndexedMemory{m}}
	fd := fakeDrift{missing: map[string]bool{"billing/gone.go": true}}
	e := New(fi, fd, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	if len(got) != 1 || len(got[0].Drift) != 1 || got[0].Drift[0].Exists {
		t.Fatalf("expected one missing-file drift annotation: %+v", got)
	}
}

func TestOutputCaps(t *testing.T) {
	var rows []index.IndexedMemory
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "a6"} {
		rows = append(rows, mem(id, model.StatusActive, []string{"billing/**"}))
	}
	for _, id := range []string{"p1", "p2", "p3"} {
		rows = append(rows, mem(id, model.StatusProvisional, []string{"billing/**"}))
	}
	fi := &fakeIndex{rows: rows}
	e := New(fi, nil, policy.Default()) // max_results 5, max_provisional 2
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	if len(got) != 5 {
		t.Fatalf("got %d results, want max_results=5", len(got))
	}
	for _, r := range got { // active fills the cap first ⇒ no provisional shown
		if r.Provisional {
			t.Fatalf("provisional %s should not appear when active fills the cap", r.Memory.ID)
		}
	}

	// With fewer active, provisional fills remaining slots up to max_provisional.
	fi.rows = append(rows[:3], rows[6:]...) // a1..a3 active + p1..p3 provisional
	got, _ = e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	nProv := 0
	for _, r := range got {
		if r.Provisional {
			nProv++
		}
	}
	if len(got) != 5 || nProv != 2 {
		t.Fatalf("got %d results (%d provisional), want 5 total with 2 provisional", len(got), nProv)
	}
}

func TestDeterministicTiebreakByID(t *testing.T) {
	// Identical on every ranking key ⇒ stable order by ID.
	fi := &fakeIndex{rows: []index.IndexedMemory{
		mem("zebra", model.StatusActive, []string{"billing/**"}),
		mem("alpha", model.StatusActive, []string{"billing/**"}),
	}}
	e := New(fi, nil, policy.Default())
	got, _ := e.Retrieve(Query{Paths: []string{"billing/x.go"}})
	if !reflect.DeepEqual(ids(got), []string{"alpha", "zebra"}) {
		t.Fatalf("tiebreak = %v, want [alpha zebra]", ids(got))
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/retrieve/ -run 'TestScope|TestStale|TestProvisional|TestRanking|TestDrift|TestOutput|TestDeterministic'`
Expected: FAIL — `New`, `Query`, `Result`, `Engine`, etc. undefined.

- [ ] **Step 3: Implement retrieve.go**

Create `internal/retrieve/retrieve.go`:

```go
// Package retrieve implements TeamMemory's precision-first, lexical retrieval
// (prd.md §11). Given an action (paths + description) it selects candidate
// memories from the index (scope-glob match or FTS match), filters by status,
// annotates anchor drift (prd.md §8.6), ranks them, and applies output caps.
package retrieve

import (
	"fmt"
	"sort"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

// Provisional framing (prd.md §5.5, §11.3).
const (
	CautionFraming     = "Possible lesson from prior work. Use as caution, not policy. Add a confirmation or contradiction if your work bears on it."
	RequestObservation = "If your work bears on this, record a `confirm` or `contradict` observation with evidence."
)

// MatchKind records why a memory entered the candidate set.
type MatchKind string

const (
	MatchScope MatchKind = "scope" // an effective-scope glob matched an action path
	MatchFTS   MatchKind = "fts"   // the description matched title/summary/guidance
)

// Query describes the action being checked.
type Query struct {
	Paths           []string // target paths of the action (e.g. the file being edited)
	Description     string   // free-text action/plan description, searched via FTS
	ProvisionalMode string   // "" uses policy; one of "never" | "related" | "always"
}

// DriftInfo annotates an anchored file that has moved on since the memory was
// recorded (prd.md §8.6).
type DriftInfo struct {
	Path           string
	Exists         bool
	CommitsChanged int    // commits touching Path since the anchor commit; -1 if unknown
	Note           string // human-facing annotation
}

// Result is one retrieved memory with its match metadata and annotations.
type Result struct {
	Memory      index.IndexedMemory
	Match       MatchKind
	Specificity int         // glob specificity of the best scope match; 0 for FTS-only
	Provisional bool        // surfaced as caution rather than trusted guidance
	Caution     string      // provisional framing; empty for active
	Request     string      // requested-observation prompt; empty for active
	Drift       []DriftInfo // anchor-drift annotations; nil if nothing drifted
}

// Index is the read surface retrieval needs (satisfied by *index.Index).
type Index interface {
	All() ([]index.IndexedMemory, error)
	SearchIDs(query string) ([]string, error)
}

// DriftSource reports anchor drift for a path relative to an anchor commit,
// against the current code repository (prd.md §8.6). Implemented by GitDrift.
type DriftSource interface {
	// Drift reports whether path still exists at HEAD and how many commits have
	// touched it since sinceCommit. commitsChanged == -1 means the count could
	// not be determined (e.g. an unknown anchor commit).
	Drift(path, sinceCommit string) (exists bool, commitsChanged int, err error)
}

// Engine answers retrieval queries against an index.
type Engine struct {
	idx   Index
	drift DriftSource // may be nil to disable anchor-drift annotation
	pol   policy.Policy
}

// New builds an Engine. drift may be nil (no anchor-drift annotation).
func New(idx Index, drift DriftSource, pol policy.Policy) *Engine {
	return &Engine{idx: idx, drift: drift, pol: pol}
}

type candidate struct {
	mem         index.IndexedMemory
	match       MatchKind
	specificity int
	ftsRank     int // position in FTS results; -1 if not an FTS match
	provisional bool
	drift       []DriftInfo
	driftScore  int // higher ⇒ more drifted ⇒ ranks lower
}

// Retrieve returns the ranked, capped set of memories relevant to q.
func (e *Engine) Retrieve(q Query) ([]Result, error) {
	all, err := e.idx.All()
	if err != nil {
		return nil, err
	}

	ftsRank := map[string]int{}
	if fq := ftsQuery(q.Description); fq != "" {
		hits, err := e.idx.SearchIDs(fq)
		if err != nil {
			return nil, err
		}
		for i, id := range hits {
			if _, dup := ftsRank[id]; !dup {
				ftsRank[id] = i
			}
		}
	}

	mode := q.ProvisionalMode
	if mode == "" {
		mode = e.pol.Retrieval.ProvisionalMode
	}

	var active, prov []candidate
	for _, m := range all {
		if m.Status == model.StatusStale || m.Status == model.StatusRejected {
			continue // excluded from retrieval (prd.md §8.2)
		}
		spec, scopeMatch := bestSpecificity(m.EffectiveScope, q.Paths)
		fr, isFTS := ftsRank[m.ID]
		if !scopeMatch && !isFTS {
			continue
		}
		c := candidate{mem: m, specificity: spec, ftsRank: -1}
		if scopeMatch {
			c.match = MatchScope
		} else {
			c.match = MatchFTS
		}
		if isFTS {
			c.ftsRank = fr
		}
		switch m.Status {
		case model.StatusActive:
			active = append(active, c)
		case model.StatusProvisional, model.StatusContested:
			if mode == "never" {
				continue
			}
			if mode == "related" && !scopeMatch {
				continue // provisional appears only on scope match, not FTS-only
			}
			c.provisional = true
			prov = append(prov, c)
		}
	}

	e.annotateDrift(active)
	e.annotateDrift(prov)

	sortCandidates(active)
	sortCandidates(prov)

	return e.cap(active, prov), nil
}

// annotateDrift fills each candidate's drift annotations and drift score.
func (e *Engine) annotateDrift(cs []candidate) {
	if e.drift == nil {
		return
	}
	for i := range cs {
		for _, a := range cs[i].mem.Anchors {
			exists, changed, err := e.drift.Drift(a.Path, a.Commit)
			if err != nil {
				continue // drift lookup failure must not fail retrieval
			}
			switch {
			case !exists:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: false, CommitsChanged: 0, Note: noteMissing(a.Path)})
				cs[i].driftScore += 1000
			case changed < 0:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: true, CommitsChanged: -1, Note: noteUnknown(a.Path)})
				cs[i].driftScore += 50
			case changed > 0:
				cs[i].drift = append(cs[i].drift, DriftInfo{
					Path: a.Path, Exists: true, CommitsChanged: changed, Note: noteChanged(a.Path, changed)})
				cs[i].driftScore += changed
			default:
				// fresh (changed == 0): no annotation, no penalty
			}
		}
	}
}

// cap fills active results first, then provisional up to max_provisional, all
// within max_results (prd.md §11.3, §11.4).
func (e *Engine) cap(active, prov []candidate) []Result {
	maxResults := e.pol.Retrieval.MaxResults
	maxProv := e.pol.Retrieval.MaxProvisional

	var out []Result
	for _, c := range active {
		if len(out) >= maxResults {
			break
		}
		out = append(out, toResult(c))
	}
	slots := maxProv
	if rem := maxResults - len(out); rem < slots {
		slots = rem
	}
	for i := 0; i < len(prov) && slots > 0; i++ {
		out = append(out, toResult(prov[i]))
		slots--
	}
	return out
}

func toResult(c candidate) Result {
	r := Result{
		Memory:      c.mem,
		Match:       c.match,
		Specificity: c.specificity,
		Provisional: c.provisional,
		Drift:       c.drift,
	}
	if c.provisional {
		r.Caution = CautionFraming
		r.Request = RequestObservation
	}
	return r
}

// sortCandidates orders by the prd.md §11 ranking: glob specificity > status
// (active first) > enforcement > confidence > recency > anchor freshness, with
// FTS rank and ID as deterministic final tiebreakers.
func sortCandidates(cs []candidate) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.specificity != b.specificity {
			return a.specificity > b.specificity
		}
		if sa, sb := statusRank(a.mem.Status), statusRank(b.mem.Status); sa != sb {
			return sa > sb
		}
		if ea, eb := enfRank(a.mem.Enforcement), enfRank(b.mem.Enforcement); ea != eb {
			return ea > eb
		}
		if ca, cb := confRank(a.mem.Confidence), confRank(b.mem.Confidence); ca != cb {
			return ca > cb
		}
		if !a.mem.CreatedAt.Equal(b.mem.CreatedAt) {
			return a.mem.CreatedAt.After(b.mem.CreatedAt)
		}
		if a.driftScore != b.driftScore {
			return a.driftScore < b.driftScore // fresher first
		}
		if ka, kb := ftsKey(a.ftsRank), ftsKey(b.ftsRank); ka != kb {
			return ka < kb
		}
		return a.mem.ID < b.mem.ID
	})
}

func ftsKey(r int) int {
	if r < 0 {
		return 1 << 30 // non-FTS matches sort after real FTS ranks among equals
	}
	return r
}

func statusRank(s model.Status) int {
	switch s {
	case model.StatusActive:
		return 2
	case model.StatusContested, model.StatusProvisional:
		return 1
	default:
		return 0
	}
}

func enfRank(e model.Enforcement) int {
	switch e {
	case model.EnforcementRequirement:
		return 3
	case model.EnforcementWarning:
		return 2
	case model.EnforcementRecommendation:
		return 1
	default:
		return 0
	}
}

func confRank(c model.Confidence) int {
	switch c {
	case model.ConfidenceHigh:
		return 2
	case model.ConfidenceMedium:
		return 1
	default:
		return 0
	}
}

func noteChanged(path string, n int) string {
	return fmt.Sprintf("anchored file %s has changed %d commit(s) since this memory was recorded — verify it still applies, and `mark_stale` if not.", path, n)
}

func noteMissing(path string) string {
	return fmt.Sprintf("anchored file %s no longer exists — verify this memory still applies, and `mark_stale` if not.", path)
}

func noteUnknown(path string) string {
	return fmt.Sprintf("anchored commit for %s was not found in history — verify this memory still applies.", path)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/retrieve/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/retrieve/retrieve.go internal/retrieve/retrieve_test.go
git commit -m "feat(retrieve): retrieval engine — candidates, filtering, ranking, caps, provisional framing"
```

---

## Task 4: Git-backed drift source

The real `DriftSource`: existence via `git cat-file -e HEAD:<path>`, change count via `git rev-list --count <commit>..HEAD -- <path>` (the portable, pipe-free form of prd.md §8.6's `git log --oneline <commit>.. | wc -l`).

**Files:**
- Create: `internal/retrieve/drift.go`
- Test: `internal/retrieve/drift_test.go`

- [ ] **Step 1: Write the failing test against a real git history**

Create `internal/retrieve/drift_test.go`:

```go
package retrieve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mustRun(t *testing.T, r git.Runner, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// commitRepo builds a temp repo with three commits to file.go and returns the
// runner and the first commit's SHA.
func commitRepo(t *testing.T) (git.Runner, string) {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	mustRun(t, r, "init", "-q", "-b", "main")
	mustRun(t, r, "config", "user.email", "test@example.com")
	mustRun(t, r, "config", "user.name", "Test")

	writeFile(t, dir, "file.go", "v1")
	mustRun(t, r, "add", "file.go")
	mustRun(t, r, "commit", "-q", "-m", "c1")
	first := mustRun(t, r, "rev-parse", "HEAD")

	writeFile(t, dir, "file.go", "v2")
	mustRun(t, r, "commit", "-q", "-am", "c2")
	writeFile(t, dir, "file.go", "v3")
	mustRun(t, r, "commit", "-q", "-am", "c3")
	return r, first
}

func TestGitDriftCountsCommitsSinceAnchor(t *testing.T) {
	r, first := commitRepo(t)
	d := GitDrift{Git: r}

	exists, changed, err := d.Drift("file.go", first)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !exists {
		t.Fatal("file.go should exist at HEAD")
	}
	if changed != 2 { // c2 and c3 changed it since the anchor commit
		t.Fatalf("commitsChanged = %d, want 2", changed)
	}
}

func TestGitDriftMissingPath(t *testing.T) {
	r, first := commitRepo(t)
	d := GitDrift{Git: r}
	exists, _, err := d.Drift("does-not-exist.go", first)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if exists {
		t.Fatal("expected a missing path to report exists=false")
	}
}

func TestGitDriftUnknownCommit(t *testing.T) {
	r, _ := commitRepo(t)
	d := GitDrift{Git: r}
	exists, changed, err := d.Drift("file.go", "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !exists || changed != -1 { // path exists, but count is unknowable
		t.Fatalf("exists=%v changed=%d, want true/-1", exists, changed)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/retrieve/ -run TestGitDrift`
Expected: FAIL — `GitDrift` undefined.

- [ ] **Step 3: Implement drift.go**

Create `internal/retrieve/drift.go`:

```go
package retrieve

import (
	"strconv"
	"strings"
)

// gitRunner is the subset of *git.Runner that drift needs.
type gitRunner interface {
	Run(args ...string) (string, error)
}

// GitDrift computes anchor drift by shelling out to git in the code repo. It
// satisfies DriftSource. *git.Runner satisfies the embedded gitRunner.
type GitDrift struct{ Git gitRunner }

// Drift reports whether path exists at HEAD and how many commits have touched
// it since sinceCommit. A missing path short-circuits to (false, 0). An unknown
// anchor commit yields (true, -1): the file is there but the count is unknowable.
func (g GitDrift) Drift(path, sinceCommit string) (bool, int, error) {
	if _, err := g.Git.Run("cat-file", "-e", "HEAD:"+path); err != nil {
		return false, 0, nil // not present at HEAD
	}
	if sinceCommit == "" {
		return true, -1, nil
	}
	out, err := g.Git.Run("rev-list", "--count", sinceCommit+"..HEAD", "--", path)
	if err != nil {
		return true, -1, nil // unknown commit ⇒ count unknown, not a hard error
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return true, -1, nil
	}
	return true, n, nil
}
```

- [ ] **Step 4: Run the drift tests to verify they pass**

Run: `go test ./internal/retrieve/ -run TestGitDrift`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/retrieve/drift.go internal/retrieve/drift_test.go
git commit -m "feat(retrieve): git-backed anchor-drift source"
```

---

## Task 5: End-to-end retrieval against a real index

Prove the pipeline against a real ledger → real SQLite index → `Engine.Retrieve`: scope+FTS candidate selection and ranking (the decomposition doc's required Slice-4 integration test), plus real anchor-drift annotation wired through.

**Files:**
- Create: `internal/retrieve/integration_test.go`

- [ ] **Step 1: Write the integration test**

Create `internal/retrieve/integration_test.go`:

```go
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
```

- [ ] **Step 2: Run it to verify it passes**

Run: `go test ./internal/retrieve/`
Expected: PASS (all retrieve tests, including both integration tests).

- [ ] **Step 3: Run the full suite + vet**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 4: Commit**

```bash
git add internal/retrieve/integration_test.go
git commit -m "test(retrieve): end-to-end scope/FTS ranking and real-git drift annotation"
```

---

## Completion

After all tasks pass locally, push to `main` and confirm CI is green on all three OSes (per the standing slice workflow). Then update the memory files (`teammemory-slice-workflow.md` status → Slices 1–4 complete; note this was the first slice under the model-allocation workflow).

---

## Self-Review

**1. Spec coverage (prd.md §11, §8.6):**
- §11.1 Candidate set = scope-glob match ∪ FTS match → `Retrieve` candidate loop (`bestSpecificity` + `ftsRank`). ✅
- §11.2 Ranking: glob specificity > status > enforcement > confidence > recency > anchor freshness → `sortCandidates` comparator, in exactly that order. ✅
- §11.3 Provisional inclusion: `provisional_mode` (never|related|always), scope-only for `related`, capped at `max_provisional`, caution-framed, with requested-observation prompt → status switch + `cap` + `toResult`. ✅
- §11.4 Output cap `max_results` total → `cap`. ✅
- §8.6 Anchor drift: path exists? commits since anchor commit? annotate; drifted ranks lower; never auto-changes status → `GitDrift` + `annotateDrift` (only annotates, feeds `driftScore` tiebreaker, never mutates status). ✅
- §8.2 stale/rejected excluded from retrieval; contested surfaced only as caution → status filter + provisional bucket. ✅

**2. Placeholder scan:** Every code step contains complete, compilable code. No TODOs, no "add error handling," no "similar to Task N." ✅

**3. Type consistency:**
- `IndexedMemory.Anchors []model.Anchor` defined in Task 1, consumed in Task 3 (`annotateDrift` ranges `cs[i].mem.Anchors`) and Task 5. ✅
- `Index` interface (`All`, `SearchIDs`) matches the real `*index.Index` signatures (`All() ([]IndexedMemory, error)`, `SearchIDs(string) ([]string, error)`). ✅
- `DriftSource.Drift(path, sinceCommit string) (bool, int, error)` matches `GitDrift.Drift` and `fakeDrift.Drift`. ✅
- `gitRunner.Run(...string) (string, error)` matches `git.Runner.Run`. ✅
- `bestSpecificity(scope, paths []string) (int, bool)` and `globSpecificity(string) int` (Task 2) are called exactly that way in Task 3. ✅
- `New(idx Index, drift DriftSource, pol policy.Policy)` used identically in all tests. ✅

**4. Determinism:** Final ranking tiebreaker is `ID` (ULID, globally unique), via `sort.SliceStable` — order is fully determined and machine-independent. `ftsKey` keeps non-FTS entries from colliding with FTS rank 0. ✅

**5. Cross-platform:** Drift uses `git rev-list --count ... -- <path>` and `git cat-file -e HEAD:<path>` (no shell pipes, no `wc`), so it runs identically on Windows CI. ✅
</content>
</invoke>
