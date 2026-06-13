package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportOutPreservesHandAuthoredContent guards the `tm export --out FILE`
// contract (prd.md §10.4): the generated block is spliced into the file between
// its markers, so hand-authored content survives, and re-exporting refreshes
// the block in place rather than duplicating it or clobbering the file.
func TestExportOutPreservesHandAuthoredContent(t *testing.T) {
	dir := newGitRepo(t)
	if _, errb, code := runTM(t, dir, "", "init"); code != 0 {
		t.Fatalf("init: %s", errb)
	}
	if _, errb, code := runTM(t, dir, "",
		"propose", "decision",
		"--title", "use ULIDs for record ids",
		"--guidance", "prefer ULIDs",
		"--scope", "docs/**",
		"--session", "s1",
	); code != 0 {
		t.Fatalf("propose: %s", errb)
	}

	// A pre-existing AGENTS.md with hand-authored contributor guidance.
	out := filepath.Join(dir, "AGENTS.md")
	const handAuthored = "# Contributor guide\n\nAlways keep prd.md in sync.\n"
	if err := os.WriteFile(out, []byte(handAuthored), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, errb, code := runTM(t, dir, "", "export", "--format", "agents", "--out", out); code != 0 {
		t.Fatalf("export --out: %s", errb)
	}

	got := readFile(t, out)
	if !strings.Contains(got, "Always keep prd.md in sync.") {
		t.Fatalf("export --out clobbered hand-authored content:\n%s", got)
	}
	if !strings.Contains(got, "use ULIDs for record ids") {
		t.Fatalf("export --out did not write the generated block:\n%s", got)
	}

	// Re-export must refresh the block in place — content preserved, no second block.
	if _, errb, code := runTM(t, dir, "", "export", "--format", "agents", "--out", out); code != 0 {
		t.Fatalf("re-export: %s", errb)
	}
	got = readFile(t, out)
	if !strings.Contains(got, "Always keep prd.md in sync.") {
		t.Fatalf("re-export dropped hand-authored content:\n%s", got)
	}
	if n := strings.Count(got, "BEGIN TeamMemory"); n != 1 {
		t.Fatalf("re-export should keep exactly one generated block, got %d:\n%s", n, got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
