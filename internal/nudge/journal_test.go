package nudge_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func TestStoreLoadMissingReturnsEmptyJournal(t *testing.T) {
	s, err := nudge.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Load("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if j.Session != "sess-1" {
		t.Errorf("Session = %q, want sess-1", j.Session)
	}
	if len(j.Edits) != 0 || j.Turn != 0 {
		t.Errorf("fresh journal not empty: %+v", j)
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	s, err := nudge.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	j, _ := s.Load("sess-1")
	j.Turn = 4
	j.Edits = append(j.Edits, nudge.EditRecord{Path: "a/b.go", Turn: 2})
	if err := s.Save(j); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Turn != 4 || len(got.Edits) != 1 || got.Edits[0].Path != "a/b.go" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
