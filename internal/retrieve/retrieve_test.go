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
