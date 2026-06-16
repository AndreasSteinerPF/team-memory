package cli

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func TestListMatchDuplicate(t *testing.T) {
	m := index.IndexedMemory{ID: "M", Status: model.StatusDuplicate}
	if !listMatch(m, false, false, false, false, true, false, false, retrieve.GitDrift{}, 0, nil) {
		t.Fatal("--duplicate did not match a duplicate memory")
	}
	active := index.IndexedMemory{ID: "M2", Status: model.StatusActive}
	if listMatch(active, false, false, false, false, true, false, false, retrieve.GitDrift{}, 0, nil) {
		t.Fatal("--duplicate matched an active memory")
	}
}

func TestListMatchSuperseded(t *testing.T) {
	m := index.IndexedMemory{ID: "M", Status: model.StatusSuperseded}
	if !listMatch(m, false, false, false, false, false, true, false, retrieve.GitDrift{}, 0, nil) {
		t.Fatal("--superseded did not match a superseded memory")
	}
}

func TestListMatchPendingSupersede(t *testing.T) {
	m := index.IndexedMemory{ID: "B", Status: model.StatusActive}
	pending := map[string]bool{"B": true}
	if !listMatch(m, false, false, false, false, false, false, true, retrieve.GitDrift{}, 0, pending) {
		t.Fatal("--pending-supersede did not match B with pending claim")
	}
	other := index.IndexedMemory{ID: "C", Status: model.StatusActive}
	if listMatch(other, false, false, false, false, false, false, true, retrieve.GitDrift{}, 0, pending) {
		t.Fatal("--pending-supersede matched a memory with no pending claim")
	}
}

func TestListMatchDefaultExcludesNonActive(t *testing.T) {
	cases := []struct {
		name   string
		status model.Status
		want   bool
	}{
		{"active", model.StatusActive, true},
		{"stale", model.StatusStale, false},
		{"rejected", model.StatusRejected, false},
		{"duplicate", model.StatusDuplicate, false},
		{"superseded", model.StatusSuperseded, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := index.IndexedMemory{ID: "X", Status: c.status}
			got := listMatch(m, false, false, false, false, false, false, false, retrieve.GitDrift{}, 0, nil)
			if got != c.want {
				t.Fatalf("default listMatch(%s) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}
