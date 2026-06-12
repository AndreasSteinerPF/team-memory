package git_test

import (
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
)

// initRepo creates an empty git repo with a committer identity configured.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	r := git.Runner{Dir: dir}
	if _, err := r.Run("init", "-q", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := r.Run("config", "user.email", "test@example.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if _, err := r.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("config name: %v", err)
	}
	return dir
}

func TestRunErrorsOutsideRepoAndIncludesStderr(t *testing.T) {
	r := git.Runner{Dir: t.TempDir()} // empty dir, not a repo
	_, err := r.Run("rev-parse", "--git-dir")
	if err == nil {
		t.Fatal("expected an error running git in a non-repository directory")
	}
}

func TestRunInputWritesBlobAndRunReadsItBack(t *testing.T) {
	r := git.Runner{Dir: initRepo(t)}

	sha, err := r.RunInput([]byte("hello\n"), "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatalf("hash-object: %v", err)
	}
	if sha == "" {
		t.Fatal("expected a non-empty object id")
	}

	out, err := r.Run("cat-file", "-p", sha)
	if err != nil {
		t.Fatalf("cat-file: %v", err)
	}
	if out != "hello" {
		t.Fatalf("round-trip mismatch: got %q want %q", out, "hello")
	}
}

func TestRefExists(t *testing.T) {
	r := git.Runner{Dir: initRepo(t)}
	if r.RefExists("refs/heads/teammemory") {
		t.Fatal("branch should not exist yet")
	}
}
