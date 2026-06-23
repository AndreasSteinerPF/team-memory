package ledger

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestMemoryRoundTrip(t *testing.T) {
	want := model.Memory{
		ID:        "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Type:      model.TypeFailedAttempt,
		Title:     "Billing migrations require downgrade-path tests",
		Summary:   "Rollback failed when invoice_state migration lacked a downgrade path.",
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "claude-code", Email: "dev@example.com", SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := marshalMemory(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalMemory(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != want.ID || got.Type != want.Type || got.Title != want.Title {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if len(got.Scope.Paths) != 1 || got.Scope.Paths[0] != "billing/migrations/**" {
		t.Fatalf("scope mismatch: %+v", got.Scope)
	}
	if got.Actor != want.Actor {
		t.Fatalf("actor mismatch: got %+v want %+v", got.Actor, want.Actor)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("time mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestScopeCommandsRoundTrip(t *testing.T) {
	want := model.Memory{
		ID:    "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Type:  model.TypeFailedAttempt,
		Title: "Commands round-trip",
		Scope: model.Scope{
			Paths:    []string{"tests/**"},
			Commands: []string{"pytest *", "make test"},
		},
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "claude-code", SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := marshalMemory(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalMemory(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Scope.Commands) != 2 || got.Scope.Commands[0] != "pytest *" || got.Scope.Commands[1] != "make test" {
		t.Fatalf("scope.commands mismatch: got %+v want %+v", got.Scope.Commands, want.Scope.Commands)
	}
	if len(got.Scope.Paths) != 1 || got.Scope.Paths[0] != "tests/**" {
		t.Fatalf("scope.paths mismatch: got %+v want %+v", got.Scope.Paths, want.Scope.Paths)
	}
}

func TestObservationRoundTrip(t *testing.T) {
	want := model.Observation{
		ID:        "01J8X5A2P4HND7QW9XK1MZRTGE",
		Target:    "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Kind:      model.KindConfirm,
		Summary:   "Same rollback failure reproduced on revenue-reporting branch.",
		Actor:     model.Actor{Kind: model.ActorAgent, Name: "codex", Email: "reviewer@example.com", SessionID: "s2"},
		CreatedAt: time.Date(2026, 6, 15, 11, 20, 0, 0, time.UTC),
	}

	data, err := marshalObservation(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := unmarshalObservation(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != want.ID || got.Target != want.Target || got.Kind != want.Kind {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if got.Actor != want.Actor {
		t.Fatalf("actor mismatch: got %+v want %+v", got.Actor, want.Actor)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("time mismatch: got %v want %v", got.CreatedAt, want.CreatedAt)
	}
}
