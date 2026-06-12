package derive

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func TestMatchPathGlob(t *testing.T) {
	cases := []struct {
		path, glob string
		want       bool
	}{
		{"billing/migrations/2026.sql", "billing/migrations/**", true},
		{"billing/migrations/2026.sql", "billing/*.go", false},
		{"anything/at/all", "**", true},
		{"src/main.go", "src/main.go", true},
	}
	for _, c := range cases {
		if got := MatchPathGlob(c.path, c.glob); got != c.want {
			t.Errorf("MatchPathGlob(%q,%q)=%v want %v", c.path, c.glob, got, c.want)
		}
	}
}

func TestMatchPath(t *testing.T) {
	s := model.Scope{Paths: []string{"auth/**", "src/main.go"}}
	if !MatchPath("auth/login.go", s) {
		t.Error("expected auth/login.go to match scope")
	}
	if MatchPath("docs/readme.md", s) {
		t.Error("did not expect docs/readme.md to match scope")
	}
}
