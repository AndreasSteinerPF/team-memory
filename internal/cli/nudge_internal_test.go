package cli

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func TestActedOnSignalMatchesGlobScope(t *testing.T) {
	mems := []model.Memory{{
		Scope: model.Scope{Paths: []string{"internal/index/**"}},
		Actor: model.Actor{SessionID: "s1"},
	}}
	s := nudge.Signal{Type: nudge.SigChurn, Verb: "propose", Path: "internal/index/x.go"}
	if !actedOnSignal("s1", s, mems, nil) {
		t.Fatal("expected glob-scoped proposal to count as already acted")
	}
}

func TestActedOnSignalDoesNotMatchPathlessPropose(t *testing.T) {
	mems := []model.Memory{{
		Scope: model.Scope{Paths: []string{"**"}},
		Actor: model.Actor{SessionID: "s1"},
	}}
	s := nudge.Signal{Type: nudge.SigRevert, Verb: "propose"}
	if actedOnSignal("s1", s, mems, nil) {
		t.Fatal("pathless revert must not be suppressed by a catch-all path scope")
	}
}
