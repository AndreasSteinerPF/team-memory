package retrieve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func mustRun(t *testing.T, r git.Runner, args ...string) string {
	t.Helper()
	out, err := r.Run(args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// commitRepo builds a temp repo with three commits to file.go and returns the
// runner and the first commit's SHA.
func commitRepo(t *testing.T) (git.Runner, string) {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	mustRun(t, r, "init", "-q", "-b", "main")
	mustRun(t, r, "config", "user.email", "test@example.com")
	mustRun(t, r, "config", "user.name", "Test")

	writeFile(t, dir, "file.go", "v1")
	mustRun(t, r, "add", "file.go")
	mustRun(t, r, "commit", "-q", "-m", "c1")
	first := mustRun(t, r, "rev-parse", "HEAD")

	writeFile(t, dir, "file.go", "v2")
	mustRun(t, r, "commit", "-q", "-am", "c2")
	writeFile(t, dir, "file.go", "v3")
	mustRun(t, r, "commit", "-q", "-am", "c3")
	return r, first
}

func TestGitDriftCountsCommitsSinceAnchor(t *testing.T) {
	r, first := commitRepo(t)
	d := GitDrift{Git: r}

	exists, changed, err := d.Drift("file.go", first)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !exists {
		t.Fatal("file.go should exist at HEAD")
	}
	if changed != 2 { // c2 and c3 changed it since the anchor commit
		t.Fatalf("commitsChanged = %d, want 2", changed)
	}
}

func TestGitDriftMissingPath(t *testing.T) {
	r, first := commitRepo(t)
	d := GitDrift{Git: r}
	exists, _, err := d.Drift("does-not-exist.go", first)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if exists {
		t.Fatal("expected a missing path to report exists=false")
	}
}

func TestGitDriftUnknownCommit(t *testing.T) {
	r, _ := commitRepo(t)
	d := GitDrift{Git: r}
	exists, changed, err := d.Drift("file.go", "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !exists || changed != -1 { // path exists, but count is unknowable
		t.Fatalf("exists=%v changed=%d, want true/-1", exists, changed)
	}
}
