package derive

import (
	"testing"
	"time"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func ts(sec int) time.Time { return time.Date(2026, 6, 15, 10, 0, sec, 0, time.UTC) }

func TestIndependence(t *testing.T) {
	m := model.Memory{Actor: model.Actor{SessionID: "s1"}}
	same := model.Observation{Actor: model.Actor{SessionID: "s1"}}
	diff := model.Observation{Actor: model.Actor{SessionID: "s2"}}
	none := model.Observation{Actor: model.Actor{SessionID: ""}}

	if isIndependent(same, m, "different_session") {
		t.Error("same session is not independent")
	}
	if !isIndependent(diff, m, "different_session") {
		t.Error("different session is independent")
	}
	if isIndependent(none, m, "different_session") {
		t.Error("missing session id is not independent")
	}
}

func TestDifferentActorIndependenceUsesEmailWhenAvailable(t *testing.T) {
	m := model.Memory{Actor: model.Actor{SessionID: "s1", Email: "dev@example.com"}}
	sameEmail := model.Observation{Actor: model.Actor{SessionID: "s2", Email: "dev@example.com"}}
	diffEmail := model.Observation{Actor: model.Actor{SessionID: "s3", Email: "reviewer@example.com"}}
	missingEmail := model.Observation{Actor: model.Actor{SessionID: "s4"}}
	sameSessionMissingEmail := model.Observation{Actor: model.Actor{SessionID: "s1"}}

	if isIndependent(sameEmail, m, "different_actor") {
		t.Error("same actor email is not independent even across sessions")
	}
	if !isIndependent(diffEmail, m, "different_actor") {
		t.Error("different actor email is independent")
	}
	if !isIndependent(missingEmail, m, "different_actor") {
		t.Error("missing email falls back to different-session independence")
	}
	if isIndependent(sameSessionMissingEmail, m, "different_actor") {
		t.Error("missing email fallback still rejects same-session observations")
	}
}

func TestStatusProgression(t *testing.T) {
	p := policy.Default()
	// High-risk memory (migrations) — needs one independent confirm to activate.
	m := model.Memory{
		Type:      model.TypeFailedAttempt,
		Scope:     model.Scope{Paths: []string{"billing/migrations/**"}},
		Actor:     model.Actor{Kind: model.ActorAgent, SessionID: "s1"},
		CreatedAt: ts(0),
	}

	// No observations → provisional.
	st, _ := computeStatus(m, nil, model.RiskHigh, p)
	if st != model.StatusProvisional {
		t.Errorf("no obs → %q, want provisional", st)
	}

	// One independent confirm → active.
	confirm := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s2"}, CreatedAt: ts(1)}
	st, _ = computeStatus(m, []model.Observation{confirm}, model.RiskHigh, p)
	if st != model.StatusActive {
		t.Errorf("one independent confirm → %q, want active", st)
	}

	// Unresolved contradiction → contested.
	contra := model.Observation{Kind: model.KindContradict, Actor: model.Actor{SessionID: "s3"}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{confirm, contra}, model.RiskHigh, p)
	if st != model.StatusContested {
		t.Errorf("unresolved contradiction → %q, want contested", st)
	}

	// A newer confirm resolves the contradiction → active again.
	confirm2 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s4"}, CreatedAt: ts(3)}
	st, _ = computeStatus(m, []model.Observation{confirm, contra, confirm2}, model.RiskHigh, p)
	if st != model.StatusActive {
		t.Errorf("resolved contradiction → %q, want active", st)
	}

	// Reject is terminal.
	reject := model.Observation{Kind: model.KindReject, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(4)}
	st, _ = computeStatus(m, []model.Observation{confirm, reject}, model.RiskHigh, p)
	if st != model.StatusRejected {
		t.Errorf("reject → %q, want rejected", st)
	}
}

func TestLowRiskActivatesImmediately(t *testing.T) {
	p := policy.Default()
	m := model.Memory{Type: model.TypeStaleDoc, Actor: model.Actor{SessionID: "s1"}, CreatedAt: ts(0)}
	st, _ := computeStatus(m, nil, model.RiskLow, p)
	if st != model.StatusActive {
		t.Errorf("low risk, no obs → %q, want active", st)
	}
}

func TestCriticalAutoActivatesOnTwoConfirms(t *testing.T) {
	p := policy.Default()
	m := model.Memory{Type: model.TypeConstraint, Actor: model.Actor{SessionID: "s1"}, CreatedAt: ts(0)}

	// One independent confirm is not enough for critical (bar is 2).
	c1 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s2"}, CreatedAt: ts(1)}
	st, _ := computeStatus(m, []model.Observation{c1}, model.RiskCritical, p)
	if st != model.StatusProvisional {
		t.Errorf("critical with one confirm → %q, want provisional", st)
	}

	// A second independent confirm activates it.
	c2 := model.Observation{Kind: model.KindConfirm, Actor: model.Actor{SessionID: "s3"}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{c1, c2}, model.RiskCritical, p)
	if st != model.StatusActive {
		t.Errorf("critical with two independent confirms → %q, want active", st)
	}

	// A human approve still activates instantly, regardless of confirm count.
	approve := model.Observation{Kind: model.KindApprove, Actor: model.Actor{Kind: model.ActorHuman}, CreatedAt: ts(2)}
	st, _ = computeStatus(m, []model.Observation{approve}, model.RiskCritical, p)
	if st != model.StatusActive {
		t.Errorf("critical with human approve → %q, want active", st)
	}
}

func TestSuccessfulPatternProvisionalAtCreation(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeSuccessfulPattern,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	got, _ := computeStatus(m, nil, model.RiskLow, policy.Default())
	if got != model.StatusProvisional {
		t.Fatalf("successful_pattern at creation: got %q, want %q (gated)", got, model.StatusProvisional)
	}
}

func TestSuccessfulPatternActivatesOnIndependentConfirm(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeSuccessfulPattern,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{{
		Kind:  model.KindConfirm,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}, // independent
	}}
	got, _ := computeStatus(m, obs, model.RiskLow, policy.Default())
	if got != model.StatusActive {
		t.Fatalf("successful_pattern after 1 independent confirm: got %q, want %q", got, model.StatusActive)
	}
}

func TestSuccessfulPatternStaysProvisionalOnSameSessionConfirm(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeSuccessfulPattern,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{{
		Kind:  model.KindConfirm,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}, // same session — not independent
	}}
	got, _ := computeStatus(m, obs, model.RiskLow, policy.Default())
	if got != model.StatusProvisional {
		t.Fatalf("successful_pattern after same-session confirm: got %q, want %q", got, model.StatusProvisional)
	}
}

func TestMarkDuplicateAutoEffects(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{{
		Kind:        model.KindMarkDuplicate,
		Target:      "M1",
		CanonicalID: "M_CANON",
		Actor:       model.Actor{Kind: model.ActorAgent, SessionID: "s2"},
		CreatedAt:   time.Unix(100, 0),
	}}
	got, _ := computeStatus(m, obs, model.RiskLow, policy.Default())
	if got != model.StatusDuplicate {
		t.Fatalf("mark_duplicate on M1: got %q, want %q", got, model.StatusDuplicate)
	}
}

func TestMarkDuplicateReversedByLaterConfirm(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{
		{Kind: model.KindMarkDuplicate, CanonicalID: "M_CANON",
			Actor:     model.Actor{Kind: model.ActorAgent, SessionID: "s2"},
			CreatedAt: time.Unix(100, 0)},
		{Kind: model.KindConfirm,
			Actor:     model.Actor{Kind: model.ActorAgent, SessionID: "s3"},
			CreatedAt: time.Unix(200, 0)},
	}
	got, _ := computeStatus(m, obs, model.RiskLow, policy.Default())
	if got == model.StatusDuplicate {
		t.Fatal("later confirm should resolve the mark_duplicate (prd.md §8.2)")
	}
}

func TestStalePrecedesOverDuplicate(t *testing.T) {
	// Status precedence: rejected > stale > duplicate > superseded > contested > active > provisional
	m := model.Memory{ID: "M1", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{
		{Kind: model.KindMarkStale, Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}, CreatedAt: time.Unix(50, 0)},
		{Kind: model.KindMarkDuplicate, CanonicalID: "X", Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s3"}, CreatedAt: time.Unix(60, 0)},
	}
	got, _ := computeStatus(m, obs, model.RiskLow, policy.Default())
	if got != model.StatusStale {
		t.Fatalf("stale > duplicate precedence: got %q, want %q", got, model.StatusStale)
	}
}

func TestComputeStatusReturnsSupersededFromContext(t *testing.T) {
	m := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	ctx := Context{SupersededBy: map[string]string{"B": "A"}}
	got, _ := computeStatusWithContext(m, nil, model.RiskLow, policy.Default(), ctx)
	if got != model.StatusSuperseded {
		t.Fatalf("B with SupersededBy[B]=A: got %q, want %q", got, model.StatusSuperseded)
	}
}

func TestSupersededPrecedesContested(t *testing.T) {
	m := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{
		{Kind: model.KindContradict, Target: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}, CreatedAt: time.Unix(50, 0)},
	}
	ctx := Context{SupersededBy: map[string]string{"B": "A"}}
	got, _ := computeStatusWithContext(m, obs, model.RiskLow, policy.Default(), ctx)
	if got != model.StatusSuperseded {
		t.Fatalf("precedence: got %q, want %q", got, model.StatusSuperseded)
	}
}

// TestMarkDuplicateRevertsWhenCanonicalNotAlive pins R-N2 orphan revival
// (prd.md §8.5) at the per-memory derive layer: a memory with an unresolved
// mark_duplicate naming a non-alive canonical must NOT report status=duplicate.
// Real callers compute Alive via BuildContext; tests pass it directly.
func TestMarkDuplicateRevertsWhenCanonicalNotAlive(t *testing.T) {
	m := model.Memory{ID: "M1", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{{
		Kind:        model.KindMarkDuplicate,
		Target:      "M1",
		CanonicalID: "C",
		Actor:       model.Actor{Kind: model.ActorAgent, SessionID: "s2"},
		CreatedAt:   time.Unix(100, 0),
	}}
	ctx := Context{Alive: map[string]bool{"C": false, "M1": true}}
	got, _ := computeStatusWithContext(m, obs, model.RiskLow, policy.Default(), ctx)
	if got == model.StatusDuplicate {
		t.Fatalf("non-alive canonical must revert duplicate (got %q)", got)
	}
}

func TestStalePrecedesSuperseded(t *testing.T) {
	m := model.Memory{ID: "B", Type: model.TypeDecision,
		Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s1"}}
	obs := []model.Observation{
		{Kind: model.KindMarkStale, Target: "B",
			Actor: model.Actor{Kind: model.ActorAgent, SessionID: "s2"}, CreatedAt: time.Unix(50, 0)},
	}
	ctx := Context{SupersededBy: map[string]string{"B": "A"}}
	got, _ := computeStatusWithContext(m, obs, model.RiskLow, policy.Default(), ctx)
	if got != model.StatusStale {
		t.Fatalf("stale > superseded: got %q, want %q", got, model.StatusStale)
	}
}
