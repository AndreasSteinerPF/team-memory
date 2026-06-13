package nudge_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func cfg() nudge.Config {
	return nudge.Config{Enabled: true, MaxPerSession: 3, CooldownTurns: 3, SelfReviewEvery: 8, ChurnThreshold: 3}
}

func never(nudge.Signal) bool { return false }

func TestDecideEmitsPointedNudgeForFailPass(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("internal/index/x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok {
		t.Fatal("expected a nudge")
	}
	if !strings.Contains(out.Text, "tm_propose") || !strings.Contains(out.Text, "failed_attempt") {
		t.Errorf("nudge text missing propose guidance: %q", out.Text)
	}
}

func TestDecideSuppressesWhenActed(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3
	always := func(nudge.Signal) bool { return true }
	if _, ok := nudge.Decide(j, cfg(), always); ok {
		t.Error("expected suppression when the agent already acted")
	}
}

func TestDecideObserveOutranksPropose(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordSurfaced("MEM1", "a.go", false) // → observe
	j.RecordEdit("a.go")
	for i := 0; i < 3; i++ { // also churn on b.go → propose
		j.Turn = i
		j.RecordEdit("b.go")
	}
	j.Turn = 5
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok || out.Verb != "observe" {
		t.Errorf("expected observe to win, got %+v ok=%v", out, ok)
	}
}

func TestDecideRespectsCooldown(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 2}
	j.Fired = append(j.Fired, nudge.FiredNudge{Key: "prior", Turn: 1}) // 1 turn ago < cooldown 3
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}
	if _, ok := nudge.Decide(j, cfg(), never); ok {
		t.Error("expected cooldown to suppress a nudge")
	}
}

func TestDecideRespectsMaxPerSession(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 20}
	j.Fired = []nudge.FiredNudge{{Turn: 1}, {Turn: 5}, {Turn: 9}} // already 3
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}
	if _, ok := nudge.Decide(j, cfg(), never); ok {
		t.Error("expected max-per-session ceiling to suppress")
	}
}

func TestDecidePeriodicSelfReview(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 9} // 9 >= SelfReviewEvery, no prior nudge
	j.RecordEdit("a.go")                       // session has ≥1 edit
	out, ok := nudge.Decide(j, cfg(), never)
	if !ok || !strings.Contains(out.Text, "memory-worthy") {
		t.Errorf("expected a periodic self-review, got %+v ok=%v", out, ok)
	}
}
