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

// HasCycleBackTo: two-hop supersede cycle (A supersedes B, B supersedes C;
// filing C supersedes A closes the 3-cycle).
func TestHasCycleBackToTwoHopSupersede(t *testing.T) {
	obs := []model.Observation{
		{Target: "A", Kind: model.KindSupersede, Supersedes: "B"}, // A supersedes B
		{Target: "B", Kind: model.KindSupersede, Supersedes: "C"}, // B supersedes C
	}
	// Filing "C supersede --supersedes=A": call HasCycleBackTo(obs, "C", "A", ...).
	// For supersede the chain walks "is superseded by". A is superseded by ?
	// Look for obs with Supersedes=A — none. Hmm, so this test exposes the
	// direction: when filing "C supersede --supersedes=A", we're saying A is
	// now superseded by C. The chain we walk is "starting from C, who is C
	// superseded by?" — look for obs with Supersedes=C. obs[1] has Supersedes=C,
	// Target=B → so C is superseded by B. Walk to B. B is superseded by? obs[0]
	// has Supersedes=B, Target=A → B is superseded by A. Walk to A. A==a? Yes,
	// cycle.
	if !HasCycleBackTo(obs, "A", "C", model.KindSupersede) {
		t.Fatal("expected cycle on 3-step supersede chain")
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
