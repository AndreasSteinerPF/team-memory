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
