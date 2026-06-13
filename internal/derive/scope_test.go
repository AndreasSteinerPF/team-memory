package derive

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func tm(sec int) time.Time { return time.Date(2026, 6, 15, 10, 0, sec, 0, time.UTC) }

func TestScopeIsBroad(t *testing.T) {
	cases := []struct {
		glob string
		want bool
	}{
		{"billing/migrations/**", false},
		{"**", true},
		{"*/**", true},
		{"src/**", false},
		{"**/auth/**", true},
	}
	for _, c := range cases {
		if got := globIsBroad(c.glob); got != c.want {
			t.Errorf("globIsBroad(%q) = %v, want %v", c.glob, got, c.want)
		}
	}
}

func TestGlobIntersects(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"billing/migrations/**", "**/migrations/**", true},
		{"billing/migrations/**", "**/auth/**", false},
		{"docs/migrations-guide/**", "**/migrations/**", false}, // segment-exact, no false match
		{"**", "**/auth/**", true},                               // ** touches everything
		{".github/workflows/ci.yml", ".github/workflows/**", true},
	}
	for _, c := range cases {
		if got := globIntersects(c.a, c.b); got != c.want {
			t.Errorf("globIntersects(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestScopeSubset(t *testing.T) {
	outer := model.Scope{Paths: []string{"billing/migrations/**"}}
	narrower := model.Scope{Paths: []string{"billing/migrations/manual/**"}}
	broader := model.Scope{Paths: []string{"billing/**"}}

	if !scopeSubset(narrower, outer) {
		t.Error("manual/** should be subset of migrations/**")
	}
	if scopeSubset(broader, outer) {
		t.Error("billing/** should NOT be subset of migrations/**")
	}
}

func TestPathMatchesScope(t *testing.T) {
	s := model.Scope{Paths: []string{"billing/migrations/**"}}
	if !pathMatchesScope("billing/migrations/2026_add_invoice_state.sql", s) {
		t.Error("path under migrations should match")
	}
	if pathMatchesScope("billing/reports/q1.go", s) {
		t.Error("path outside migrations should not match")
	}
}

func TestEffectiveScopeNarrowsImmediately(t *testing.T) {
	m := model.Memory{
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: tm(0),
	}
	obs := []model.Observation{{
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Paths: []string{"billing/migrations/manual/**"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      tm(1),
	}}
	got := effectiveScope(m, obs)
	if len(got.Paths) != 1 || got.Paths[0] != "billing/migrations/manual/**" {
		t.Fatalf("narrowing should apply immediately, got %v", got.Paths)
	}
}

func TestCommandContains(t *testing.T) {
	cases := []struct {
		outer, inner string
		want         bool
	}{
		{"assistant *", "assistant jira create *", true},  // broader contains narrower
		{"assistant jira create *", "assistant *", false}, // narrower does not contain broader
		{"assistant *", "assistant *", true},              // reflexive
		{"pytest", "pytest", true},                        // exact no-star reflexive
		{"assistant jira *", "assistant billing *", false},
	}
	for _, c := range cases {
		if got := commandContains(c.outer, c.inner); got != c.want {
			t.Errorf("commandContains(%q, %q) = %v, want %v", c.outer, c.inner, got, c.want)
		}
	}
}

func TestEffectiveScopeNarrowsCommands(t *testing.T) {
	m := model.Memory{
		ID:        "01M",
		Type:      model.TypeConstraint,
		Scope:     model.Scope{Commands: []string{"assistant *"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
	}
	adj := model.Observation{
		Target:         "01M",
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Commands: []string{"assistant jira create *"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC),
	}
	got := effectiveScope(m, []model.Observation{adj})
	if len(got.Commands) != 1 || got.Commands[0] != "assistant jira create *" {
		t.Fatalf("effective commands = %v, want [assistant jira create *] (narrowing applies immediately)", got.Commands)
	}
}

func TestEffectiveScopeBroadeningNeedsSubstantiation(t *testing.T) {
	m := model.Memory{
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{SessionID: "s1"},
		CreatedAt: tm(0),
	}
	adjust := model.Observation{
		Kind:           model.KindAdjustScope,
		SuggestedScope: &model.Scope{Paths: []string{"billing/**"}},
		Actor:          model.Actor{SessionID: "s2"},
		CreatedAt:      tm(1),
	}

	// Unsubstantiated broadening does not apply.
	if got := effectiveScope(m, []model.Observation{adjust}); got.Paths[0] != "billing/migrations/**" {
		t.Errorf("unsubstantiated broadening should NOT apply, got %v", got.Paths)
	}

	// A later independent confirm touching the broadened-but-not-prior area substantiates it.
	confirm := model.Observation{
		Kind:        model.KindConfirm,
		Actor:       model.Actor{SessionID: "s3"},
		CodeContext: &model.CodeContext{Paths: []string{"billing/reports/q1.go"}},
		CreatedAt:   tm(2),
	}
	if got := effectiveScope(m, []model.Observation{adjust, confirm}); got.Paths[0] != "billing/**" {
		t.Errorf("substantiated broadening should apply, got %v", got.Paths)
	}
}
