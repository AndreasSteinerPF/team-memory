package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/sessionid"
)

// TestEnvSessionPrefersEnvVar pins the precedence: $CLAUDE_SESSION_ID wins
// over the file fallback when both are present.
func TestEnvSessionPrefersEnvVar(t *testing.T) {
	dir := t.TempDir()
	chdirTemp(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "tm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "tm", "current-session.txt"), []byte("file-sess\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_SESSION_ID", "env-sess")
	if got := envSession(); got != "env-sess" {
		t.Fatalf("envSession = %q, want %q", got, "env-sess")
	}
}

// TestEnvSessionFallsBackToFile covers the Claude Code case: the env var is
// not exported into the Bash tool's subprocess, so envSession must recover
// the session id from .git/tm/current-session.txt (written by the signal/
// nudge hooks).
func TestEnvSessionFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	chdirTemp(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "tm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "tm", "current-session.txt"), []byte("file-sess\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_SESSION_ID", "")
	if got := envSession(); got != "file-sess" {
		t.Fatalf("envSession = %q, want %q", got, "file-sess")
	}
}

// TestEnvSessionWalksUpToRepoRoot proves the file lookup walks from a nested
// CWD (the repo root may be several directories above where the CLI runs).
func TestEnvSessionWalksUpToRepoRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git", "tm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "tm", "current-session.txt"), []byte("repo-sess\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	chdirTemp(t, nested)
	t.Setenv("CLAUDE_SESSION_ID", "")
	if got := envSession(); got != "repo-sess" {
		t.Fatalf("envSession = %q, want %q", got, "repo-sess")
	}
}

// TestEnvSessionEmptyWhenNoSources guarantees the helper degrades silently
// when neither the env var nor the file are set — callers handle the empty
// string by falling back to TTL semantics (prd.md §10.2).
func TestEnvSessionEmptyWhenNoSources(t *testing.T) {
	dir := t.TempDir()
	chdirTemp(t, dir)
	t.Setenv("CLAUDE_SESSION_ID", "")
	if got := envSession(); got != "" {
		t.Fatalf("envSession = %q, want empty", got)
	}
}

// TestWriteCurrentSessionRoundtrip pins the write/read pair the signal/nudge
// hooks rely on. The file must be created (with the .git/tm subdir) and read
// back via currentSessionFromFile() unchanged.
func TestWriteCurrentSessionRoundtrip(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCurrentSession(gitDir, "abc-123")
	chdirTemp(t, repo)
	if got := sessionid.FromFile(); got != "abc-123" {
		t.Fatalf("sessionid.FromFile = %q, want %q", got, "abc-123")
	}
}

// TestWriteCurrentSessionEmptyIsNoop guards against accidentally clobbering a
// previously-recorded session with the empty string from a payload that
// omitted session_id.
func TestWriteCurrentSessionEmptyIsNoop(t *testing.T) {
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "tm"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(gitDir, "tm", "current-session.txt")
	if err := os.WriteFile(path, []byte("keep-me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCurrentSession(gitDir, "")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "keep-me\n" {
		t.Fatalf("file was overwritten by empty session id: %q", b)
	}
}

// chdirTemp changes the working directory for the duration of the test,
// restoring the previous CWD via t.Cleanup so other tests aren't affected.
func chdirTemp(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}
