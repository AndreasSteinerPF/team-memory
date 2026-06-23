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

func TestDecideReturnsFiredMetadata(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test ./...", true)
	j.Turn = 2
	j.RecordEdit("internal/index/x.go")
	j.RecordCommand("go test ./...", false)
	j.Turn = 3

	dec := nudge.Decide(j, cfg(), never)
	if !dec.Fired {
		t.Fatal("expected a fired nudge")
	}
	if dec.Nudge.Type != nudge.SigFailPass {
		t.Fatalf("Type = %q, want %q", dec.Nudge.Type, nudge.SigFailPass)
	}
	if dec.Nudge.Verb != "propose" {
		t.Fatalf("Verb = %q, want propose", dec.Nudge.Verb)
	}
	if dec.Nudge.Path != "internal/index/x.go" {
		t.Fatalf("Path = %q, want internal/index/x.go", dec.Nudge.Path)
	}
	if dec.Nudge.MemoryID != "" {
		t.Fatalf("MemoryID = %q, want empty", dec.Nudge.MemoryID)
	}
	if dec.Nudge.Key == "" || dec.Nudge.Text == "" {
		t.Fatalf("nudge missing key/text: %+v", dec.Nudge)
	}
}

func TestDecideRecordsCooldownSuppression(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 2}
	j.Fired = append(j.Fired, nudge.FiredNudge{Key: "prior", Turn: 1})
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}

	dec := nudge.Decide(j, cfg(), never)
	if dec.Fired {
		t.Fatalf("expected no fired nudge: %+v", dec)
	}
	if len(dec.Suppressions) != 1 {
		t.Fatalf("Suppressions = %+v, want one cooldown suppression", dec.Suppressions)
	}
	if dec.Suppressions[0].Reason != nudge.SuppressCooldown {
		t.Fatalf("Reason = %q, want %q", dec.Suppressions[0].Reason, nudge.SuppressCooldown)
	}
	if dec.Suppressions[0].Type != nudge.SigChurn || dec.Suppressions[0].Path != "hot.go" {
		t.Fatalf("suppression metadata mismatch: %+v", dec.Suppressions[0])
	}
}

func TestDecideRecordsAlreadyActedSuppression(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3

	always := func(nudge.Signal) bool { return true }
	dec := nudge.Decide(j, cfg(), always)
	if dec.Fired {
		t.Fatalf("expected no fired nudge: %+v", dec)
	}
	if len(dec.Suppressions) != 1 {
		t.Fatalf("Suppressions = %+v, want one already_acted suppression", dec.Suppressions)
	}
	if dec.Suppressions[0].Reason != nudge.SuppressAlreadyActed {
		t.Fatalf("Reason = %q, want %q", dec.Suppressions[0].Reason, nudge.SuppressAlreadyActed)
	}
}

func TestDecideEmitsPointedNudgeForFailPass(t *testing.T) {
	j := &nudge.Journal{Session: "s"}
	j.Turn = 1
	j.RecordCommand("go test", true)
	j.Turn = 2
	j.RecordEdit("internal/index/x.go")
	j.RecordCommand("go test", false)
	j.Turn = 3
	dec := nudge.Decide(j, cfg(), never)
	out, ok := dec.Nudge, dec.Fired
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
	if dec := nudge.Decide(j, cfg(), always); dec.Fired {
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
	dec := nudge.Decide(j, cfg(), never)
	out, ok := dec.Nudge, dec.Fired
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
	if dec := nudge.Decide(j, cfg(), never); dec.Fired {
		t.Error("expected cooldown to suppress a nudge")
	}
}

func TestDecideRespectsMaxPerSession(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 20}
	j.Fired = []nudge.FiredNudge{{Turn: 1}, {Turn: 5}, {Turn: 9}} // already 3
	for i := 0; i < 3; i++ {
		j.RecordEdit("hot.go")
	}
	if dec := nudge.Decide(j, cfg(), never); dec.Fired {
		t.Error("expected max-per-session ceiling to suppress")
	}
}

func TestDecidePeriodicSelfReview(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 9} // 9 >= SelfReviewEvery, no prior nudge
	j.RecordEdit("a.go")                       // session has ≥1 edit
	dec := nudge.Decide(j, cfg(), never)
	out, ok := dec.Nudge, dec.Fired
	if !ok || !strings.Contains(out.Text, "memory-worthy") {
		t.Errorf("expected a periodic self-review, got %+v ok=%v", out, ok)
	}
}
