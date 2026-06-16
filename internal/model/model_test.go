package model

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestMemoryYAMLRoundTrip(t *testing.T) {
	in := Memory{
		ID:      "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Type:    TypeFailedAttempt,
		Title:   "Billing migrations require downgrade-path tests",
		Summary: "rollback failed",
		Scope:   Scope{Paths: []string{"billing/migrations/**"}},
		CodeContext: &CodeContext{
			Branch: "feature/invoice-state",
			Commit: "abc123def",
		},
		Actor:     Actor{Kind: ActorAgent, Name: "claude-code", SessionID: "session_123"},
		CreatedAt: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
	}

	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Memory
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != in.ID || out.Type != in.Type ||
		out.CodeContext == nil || out.CodeContext.Branch != "feature/invoice-state" ||
		len(out.Scope.Paths) != 1 || out.Scope.Paths[0] != "billing/migrations/**" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestObservationCarriesKindFields(t *testing.T) {
	o := Observation{
		ID:             "01J8X5A2P4HND7QW9XK1MZRTGE",
		Target:         "01J8X4QZ7M9FKE2V3R5T8WYBCD",
		Kind:           KindAdjustScope,
		SuggestedScope: &Scope{Paths: []string{"billing/migrations/manual/**"}},
		Actor:          Actor{Kind: ActorAgent, Name: "codex", SessionID: "session_456"},
		CreatedAt:      time.Now(),
	}
	data, _ := yaml.Marshal(o)
	var out Observation
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SuggestedScope == nil || out.SuggestedScope.Paths[0] != "billing/migrations/manual/**" {
		t.Fatalf("suggested_scope lost: %+v", out)
	}
}

func TestNewConstantsExist(t *testing.T) {
	if TypeSuccessfulPattern != "successful_pattern" {
		t.Fatalf("TypeSuccessfulPattern wrong: %q", TypeSuccessfulPattern)
	}
	if KindMarkDuplicate != "mark_duplicate" {
		t.Fatalf("KindMarkDuplicate wrong: %q", KindMarkDuplicate)
	}
	if KindSupersede != "supersede" {
		t.Fatalf("KindSupersede wrong: %q", KindSupersede)
	}
	if StatusDuplicate != "duplicate" {
		t.Fatalf("StatusDuplicate wrong: %q", StatusDuplicate)
	}
	if StatusSuperseded != "superseded" {
		t.Fatalf("StatusSuperseded wrong: %q", StatusSuperseded)
	}
}

func TestObservationHasCrossMemoryFields(t *testing.T) {
	o := Observation{CanonicalID: "abc", Supersedes: "def"}
	if o.CanonicalID != "abc" || o.Supersedes != "def" {
		t.Fatal("CanonicalID/Supersedes fields not present or not assignable")
	}
}
