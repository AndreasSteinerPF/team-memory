package cli

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// TestProposeAcceptsSuccessfulPattern guards the CLI propose validator: the
// successful_pattern memory type (prd.md §5.2) must be accepted so the
// activation gate (§8.2) — not the surface — controls promotion.
func TestProposeAcceptsSuccessfulPattern(t *testing.T) {
	if !validType(model.TypeSuccessfulPattern) {
		t.Fatal("validType rejected successful_pattern")
	}
}
