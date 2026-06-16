package derive

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func TestBuildContextSupersededByHumanApprove(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
		{ID: "O2", Target: "A", Kind: model.KindApprove,
			Actor: model.Actor{Kind: model.ActorHuman, Name: "alice"}, CreatedAt: time.Unix(200, 0)},
	}
	ctx := BuildContext([]model.Memory{a, b}, obs, policy.Default())
	if got, ok := ctx.SupersededBy["B"]; !ok || got != "A" {
		t.Fatalf("SupersededBy[B]: got (%q, %v), want (\"A\", true)", got, ok)
	}
}

func TestBuildContextNotSupersededWithoutSubstantiation(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
	}
	ctx := BuildContext([]model.Memory{a, b}, obs, policy.Default())
	if _, ok := ctx.SupersededBy["B"]; ok {
		t.Fatal("SupersededBy[B] set without substantiation")
	}
	if got := ctx.PendingSupersedeFor("B"); len(got) != 1 {
		t.Fatalf("PendingSupersedeFor(B): got %d, want 1", len(got))
	}
}

func TestBuildContextSupersededByIndependentConfirmOnA(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
		{ID: "O2", Target: "A", Kind: model.KindConfirm,
			Actor:     model.Actor{Kind: model.ActorAgent, SessionID: "s4"},
			CreatedAt: time.Unix(200, 0)},
	}
	ctx := BuildContext([]model.Memory{a, b}, obs, policy.Default())
	if got, ok := ctx.SupersededBy["B"]; !ok || got != "A" {
		t.Fatalf("SupersededBy[B]: got (%q, %v), want (\"A\", true)", got, ok)
	}
}

// TestBuildContextSkipsSupersedeWhenCanonicalRejected pins R-N2 orphan revival:
// when canonical A is rejected, the supersede claim against B does not
// substantiate (B is not in SupersededBy) and is not pending either — the claim
// is moot until A is revived or a fresh supersede is filed on a different A.
func TestBuildContextSkipsSupersedeWhenCanonicalRejected(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
		// Independent confirm would normally substantiate, but A is rejected.
		{ID: "O2", Target: "A", Kind: model.KindConfirm,
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s4"}, CreatedAt: time.Unix(150, 0)},
		{ID: "O3", Target: "A", Kind: model.KindReject,
			Actor: model.Actor{Kind: model.ActorHuman, Name: "alice"}, CreatedAt: time.Unix(200, 0)},
	}
	ctx := BuildContext([]model.Memory{a, b}, obs, policy.Default())
	if _, ok := ctx.SupersededBy["B"]; ok {
		t.Fatal("rejected canonical must not substantiate supersede")
	}
	if got := ctx.PendingSupersedeFor("B"); len(got) != 0 {
		t.Fatalf("rejected canonical: supersede should not be pending either, got %d", len(got))
	}
	if ctx.Alive["A"] {
		t.Fatal("rejected canonical should not be Alive")
	}
}

// TestBuildContextAliveFlipsOnStaleConfirmCycle pins the reversibility
// property of orphan revival (prd.md §8.5): once a mark_stale on A is itself
// resolved by a later confirm, A is alive again and the supersede claim
// substantiates. So B's status: superseded → reverted → superseded across
// the stale → confirm sequence.
func TestBuildContextAliveFlipsOnStaleConfirmCycle(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	supersedeObs := model.Observation{
		ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0),
	}
	initialConfirm := model.Observation{
		ID: "O2", Target: "A", Kind: model.KindConfirm,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s4"}, CreatedAt: time.Unix(150, 0),
	}
	staleObs := model.Observation{
		ID: "O3", Target: "A", Kind: model.KindMarkStale,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s5"}, CreatedAt: time.Unix(200, 0),
	}
	revivalConfirm := model.Observation{
		ID: "O4", Target: "A", Kind: model.KindConfirm,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s6"}, CreatedAt: time.Unix(300, 0),
	}

	// Step 1: supersede + confirm → B is superseded.
	step1 := []model.Observation{supersedeObs, initialConfirm}
	ctx1 := BuildContext([]model.Memory{a, b}, step1, policy.Default())
	if got, ok := ctx1.SupersededBy["B"]; !ok || got != "A" {
		t.Fatalf("step 1: SupersededBy[B] = (%q, %v), want (A, true)", got, ok)
	}

	// Step 2: + mark_stale → A not alive, B reverts (not in SupersededBy).
	step2 := []model.Observation{supersedeObs, initialConfirm, staleObs}
	ctx2 := BuildContext([]model.Memory{a, b}, step2, policy.Default())
	if _, ok := ctx2.SupersededBy["B"]; ok {
		t.Fatal("step 2 (stale): B should revert from SupersededBy")
	}
	if ctx2.Alive["A"] {
		t.Fatal("step 2 (stale): A should not be Alive")
	}

	// Step 3: + revival confirm resolves the mark_stale → A alive, B re-flips.
	step3 := []model.Observation{supersedeObs, initialConfirm, staleObs, revivalConfirm}
	ctx3 := BuildContext([]model.Memory{a, b}, step3, policy.Default())
	if !ctx3.Alive["A"] {
		t.Fatal("step 3 (revival): A should be Alive again after confirm resolves the stale")
	}
	if got, ok := ctx3.SupersededBy["B"]; !ok || got != "A" {
		t.Fatalf("step 3 (revival): B should re-flip to superseded; SupersededBy[B] = (%q, %v)", got, ok)
	}
}

// TestBuildContextSkipsSupersedeWhenCanonicalStale pins R-N2 for the unresolved
// mark_stale case: even if a confirm previously substantiated the supersede,
// an unresolved mark_stale on A reverts B (A is not alive, supersede is moot).
func TestBuildContextSkipsSupersedeWhenCanonicalStale(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	b := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
		{ID: "O2", Target: "A", Kind: model.KindConfirm,
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s4"}, CreatedAt: time.Unix(150, 0)},
		{ID: "O3", Target: "A", Kind: model.KindMarkStale,
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s5"}, CreatedAt: time.Unix(200, 0)},
	}
	ctx := BuildContext([]model.Memory{a, b}, obs, policy.Default())
	if _, ok := ctx.SupersededBy["B"]; ok {
		t.Fatal("stale canonical must not substantiate supersede")
	}
	if got := ctx.PendingSupersedeFor("B"); len(got) != 0 {
		t.Fatalf("stale canonical: supersede should not be pending, got %d", len(got))
	}
	if ctx.Alive["A"] {
		t.Fatal("canonical under unresolved mark_stale should not be Alive")
	}
}

func TestBuildContextSelfReferenceIgnored(t *testing.T) {
	a := model.Memory{ID: "A", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{
		{ID: "O1", Target: "A", Kind: model.KindSupersede, Supersedes: "A",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(100, 0)},
	}
	ctx := BuildContext([]model.Memory{a}, obs, policy.Default())
	if _, ok := ctx.SupersededBy["A"]; ok {
		t.Fatal("self-reference must not produce a supersession entry")
	}
}

// HasCycleBackTo: one-hop cycle (A→B, B→A). Same node both sides.
func TestHasCycleBackToOneHopDuplicate(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindMarkDuplicate, CanonicalID: "B"},
	}
	// Filing "B mark_duplicate canonical=A": closes A↔B. Walk from A; reach B? No,
	// the walk is from b (the canonical we're naming). Caller passes (a, b) =
	// (target_of_new_obs, canonical_of_new_obs). Filing "B mark_duplicate
	// canonical=A" means new obs's target is B, canonical is A — so call
	// HasCycleBackTo(obs, "B", "A", KindMarkDuplicate). Walk from A: A has an
	// outgoing duplicate to B. B==a? yes. Cycle.
	if !HasCycleBackTo(obs, "B", "A", model.KindMarkDuplicate) {
		t.Fatal("expected cycle when filing B→A with existing A→B")
	}
}

// HasCycleBackTo: two-hop cycle (A→B, B→C; filing C→A closes the 3-cycle).
func TestHasCycleBackToTwoHopDuplicate(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindMarkDuplicate, CanonicalID: "B"},
		{Target: "B", Kind: model.KindMarkDuplicate, CanonicalID: "C"},
	}
	// Filing "C mark_duplicate canonical=A": call HasCycleBackTo(obs, "C", "A", ...).
	// Walk from A: A→B→C. C==a? yes. Cycle.
	if !HasCycleBackTo(obs, "C", "A", model.KindMarkDuplicate) {
		t.Fatal("expected cycle on 3-step duplicate chain")
	}
}

// HasCycleBackTo: no cycle when chain doesn't close.
func TestHasCycleBackToNoCycle(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindMarkDuplicate, CanonicalID: "B"},
		{Target: "B", Kind: model.KindMarkDuplicate, CanonicalID: "C"},
	}
	// Filing "D mark_duplicate canonical=A": no path from A back to D.
	if HasCycleBackTo(obs, "D", "A", model.KindMarkDuplicate) {
		t.Fatal("unexpected cycle for D→A with chain A→B→C")
	}
}

// HasCycleBackTo: two-hop supersede cycle. Existing arcs: A supersedes B
// (obs target=A, supersedes=B → B is superseded by A → arc B→A in the
// supersession graph) and B supersedes C (arc C→B). Filing "C supersede
// --supersedes=A" creates arc A→C, closing the 3-cycle A→C→B→A. Caller
// passes (a=target=C, b=supersedes=A).
func TestHasCycleBackToTwoHopSupersede(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindSupersede, Supersedes: "B"}, // B→A
		{Target: "B", Kind: model.KindSupersede, Supersedes: "C"}, // C→B
	}
	// Walker starts at a=C, walks "is superseded by" forward: C→B→A. A==target=A.
	if !HasCycleBackTo(obs, "C", "A", model.KindSupersede) {
		t.Fatal("expected cycle on 3-step supersede chain (C→B→A)")
	}
}

// HasCycleBackTo: self-loop is detected.
func TestHasCycleBackToSelf(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindMarkDuplicate, CanonicalID: "B"},
	}
	// Filing B mark_duplicate canonical=B — would create a self-loop, but our
	// validators already block self-reference. This test ensures HasCycleBackTo
	// doesn't infinite-loop if such an observation ever sneaks through.
	obs2 := append(obs, model.Observation{Target: "B", Kind: model.KindMarkDuplicate, CanonicalID: "B"})
	_ = HasCycleBackTo(obs2, "X", "B", model.KindMarkDuplicate) // must terminate
}
