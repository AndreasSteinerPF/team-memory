package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Packaging file-content assertions live in the harness E2E packaging tier
// (e2e/harness/packaging_test.go, descriptor Packaging()). This file keeps only
// behaviors not covered there: unknown-harness error and GEMINI.md preservation.

func TestInstallUnknownHarnessErrors(t *testing.T) {
	repo := initRepo(t)
	if code := runTMLocal(t, repo, "init", "--harness", "bogus"); code == 0 {
		t.Error("expected non-zero exit for unknown harness")
	}
}

func TestInstallGeminiPreservesExistingBrief(t *testing.T) {
	repo := initRepo(t)
	// Pre-existing GEMINI.md with user content must survive.
	sentinel := "# My project rules\nAlways run the linter.\n"
	if err := os.WriteFile(filepath.Join(repo, "GEMINI.md"), []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runTMLocal(t, repo, "init", "--harness", "gemini"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got, err := os.ReadFile(filepath.Join(repo, "GEMINI.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Always run the linter.") {
		t.Error("existing GEMINI.md content was clobbered")
	}
	if !strings.Contains(string(got), "# TeamMemory") {
		t.Error("TeamMemory section not appended")
	}
}
