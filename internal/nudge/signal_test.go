package nudge_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func has(sigs []nudge.Signal, t nudge.SignalType) *nudge.Signal {
	for i := range sigs {
		if sigs[i].Type == t {
			return &sigs[i]
		}
	}
	return nil
}

func TestDetectFailPassRequiresEditBetween(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	// fail at turn 1, edit, pass at turn 2 → signal
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test ./...", true)
	j.Turn = 2
	j.RecordEdit("internal/index/index.go")
	j.RecordCommand("go test ./...", false)
	if has(nudge.Detect(j, cfg), nudge.SigFailPass) == nil {
		t.Error("expected fail_pass signal")
	}

	// fail then pass with NO edit between → no signal (transient)
	j2 := &nudge.Journal{Session: "s"}
	j2.Turn = 1
	j2.RecordCommand("go test ./...", true)
	j2.RecordCommand("go test ./...", false)
	if has(nudge.Detect(j2, cfg), nudge.SigFailPass) != nil {
		t.Error("did not expect fail_pass without an edit between")
	}
}

func TestDetectChurn(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s"}
	for i := 0; i < 3; i++ {
		j.Turn = i
		j.RecordEdit("hot.go")
	}
	if has(nudge.Detect(j, cfg), nudge.SigChurn) == nil {
		t.Error("expected churn signal at threshold 3")
	}
}

func TestDetectRevert(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordCommand("git reset --hard HEAD~1", false)
	if has(nudge.Detect(j, cfg), nudge.SigRevert) == nil {
		t.Error("expected revert signal")
	}
}

func TestDetectSurfacedButUnobserved(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM1", "billing/migrations/x.sql", false)
	j.RecordEdit("billing/migrations/x.sql")
	s := has(nudge.Detect(j, cfg), nudge.SigUnobserved)
	if s == nil || s.Memory != "MEM1" {
		t.Errorf("expected unobserved signal for MEM1, got %+v", s)
	}
}

func TestDetectDriftAnchorEdited(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM2", "core/api.go", true) // drift=true
	j.RecordEdit("core/api.go")
	if has(nudge.Detect(j, cfg), nudge.SigDrift) == nil {
		t.Error("expected drift signal when a drifted anchor is edited")
	}
}

func TestDetectUserIntervened(t *testing.T) {
	cfg := nudge.Config{ChurnThreshold: 3}
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordEdit("auth/login.go")
	j.Turn = 2
	j.RecordPrompt()
	j.Turn = 3
	j.RecordEdit("auth/login.go")
	s := has(nudge.Detect(j, cfg), nudge.SigIntervened)
	if s == nil || s.Path != "auth/login.go" {
		t.Errorf("expected intervened signal for auth/login.go, got %+v", s)
	}
}
