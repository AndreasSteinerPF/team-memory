package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// newGitRepo creates a temp dir initialized as a git repo with a committer
// identity, and returns its path.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		gitExec(t, dir, args...)
	}
	return dir
}

// gitExec runs git in dir and fails the test on error.
func gitExec(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// writeFile writes content to dir/rel, creating parent directories.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runTM runs the CLI against repoDir with the given stdin and args, returning
// stdout, stderr, and the exit code.
func runTM(t *testing.T, repoDir, stdin string, args ...string) (string, string, int) {
	t.Helper()
	full := append([]string{"--repo", repoDir}, args...)
	var out, errb bytes.Buffer
	code := cli.Run(full, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

var ulidRe = regexp.MustCompile(`[0-9A-HJKMNP-TV-Z]{26}`)

// parseID extracts the first ULID from s (e.g. propose's first output line).
func parseID(t *testing.T, s string) string {
	t.Helper()
	id := ulidRe.FindString(s)
	if id == "" {
		t.Fatalf("no ULID found in output:\n%s", s)
	}
	return id
}
