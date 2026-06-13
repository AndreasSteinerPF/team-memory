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

func TestRecordEditCounts(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordEdit("x.go")
	j.RecordEdit("x.go")
	if n := j.EditCount("x.go"); n != 2 {
		t.Errorf("EditCount = %d, want 2", n)
	}
}

func TestRecordCommandSignature(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 1}
	j.RecordCommand("go test ./...", true)
	if len(j.Commands) != 1 || j.Commands[0].Signature != "go test" || !j.Commands[0].Failed {
		t.Errorf("command not recorded: %+v", j.Commands)
	}
}

func TestRecordCommandDetectsRevert(t *testing.T) {
	j := &nudge.Journal{Session: "s", Turn: 5}
	j.RecordCommand("git reset --hard HEAD~1", false)
	if len(j.Reverts) != 1 || j.Reverts[0] != 5 {
		t.Errorf("revert not recorded: %+v", j.Reverts)
	}
}
