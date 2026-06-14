package cli_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

// initRepo creates a temp git repo and runs `tm init`, returning the repo dir.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "tm@example.com"},
		{"config", "user.name", "TM Test"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", dir, "init"}, strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("tm init failed (%d): %s", code, errb.String())
	}
	return dir
}

// runSignalForTest feeds a PostToolUse event to `tm signal --hook`, asserting success.
func runSignalForTest(t *testing.T, repo, stdin string) {
	t.Helper()
	var out, errb bytes.Buffer
	if code := cli.Run([]string{"--repo", repo, "signal", "--hook"}, strings.NewReader(stdin), &out, &errb); code != 0 {
		t.Fatalf("signal hook failed (%d): %s", code, errb.String())
	}
}
