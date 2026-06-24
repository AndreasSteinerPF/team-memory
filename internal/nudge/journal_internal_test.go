package nudge

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveRenameFailurePreservesPreviousJournal(t *testing.T) {
	gitDir := t.TempDir()
	s, err := Open(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(&Journal{Session: "s1", Turn: 1}); err != nil {
		t.Fatal(err)
	}

	s.rename = func(string, string) error { return errors.New("rename failed") }
	if err := s.Save(&Journal{Session: "s1", Turn: 2}); err == nil {
		t.Fatal("expected rename failure")
	}

	data, err := os.ReadFile(filepath.Join(gitDir, "tm", "nudge", "s1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Journal
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("previous journal was corrupted: %v", err)
	}
	if got.Turn != 1 {
		t.Fatalf("Turn = %d, want preserved value 1", got.Turn)
	}
}
