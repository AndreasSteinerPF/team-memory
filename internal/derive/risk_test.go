package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func TestRiskForScope(t *testing.T) {
	p := policy.Default()

	cases := []struct {
		name  string
		mem   model.Memory
		scope model.Scope
		want  model.Risk
	}{
		{
			name:  "failed_attempt in migrations escalates to high",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"billing/migrations/**"}},
			want:  model.RiskHigh,
		},
		{
			name:  "stale_doc stays low",
			mem:   model.Memory{Type: model.TypeStaleDoc},
			scope: model.Scope{Paths: []string{"docs/setup.md"}},
			want:  model.RiskLow,
		},
		{
			name:  "external constraint is at least high",
			mem:   model.Memory{Type: model.TypeConstraint, Origin: model.OriginExternal},
			scope: model.Scope{Paths: []string{"api/handlers.go"}},
			want:  model.RiskHigh,
		},
		{
			name:  "auth path is critical",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"internal/auth/**"}},
			want:  model.RiskCritical,
		},
		{
			// "**" is broad (medium→high) AND, being the catch-all, intersects
			// every sensitive path — including auth (critical). Catch-all wins.
			name:  "catch-all scope is critical",
			mem:   model.Memory{Type: model.TypeFailedAttempt},
			scope: model.Scope{Paths: []string{"**"}},
			want:  model.RiskCritical,
		},
	}
	for _, c := range cases {
		if got := riskForScope(c.mem, c.scope, p); got != c.want {
			t.Errorf("%s: risk = %q, want %q", c.name, got, c.want)
		}
	}
}
