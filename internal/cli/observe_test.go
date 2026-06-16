package cli

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// TestObserveAcceptsMarkDuplicateAndSupersede guards the CLI observe validator:
// the mark_duplicate and supersede observation kinds (prd.md §5.3) must be
// accepted so the cross-memory relationships (§8.2, §9.2) — not the surface —
// control their effect.
func TestObserveAcceptsMarkDuplicateAndSupersede(t *testing.T) {
	if !validAgentKind(model.KindMarkDuplicate) {
		t.Fatal("validAgentKind rejected mark_duplicate")
	}
	if !validAgentKind(model.KindSupersede) {
		t.Fatal("validAgentKind rejected supersede")
	}
}
