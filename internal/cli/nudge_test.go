package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func runNudge(t *testing.T, repo, stdin string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "nudge", "--hook"}, strings.NewReader(stdin), &out, &errb)
	return out.String(), code
}

func TestNudgeHookEmitsAfterFailPass(t *testing.T) {
	repo := initRepo(t)
	feed := func(s string) { runSignalForTest(t, repo, s) }
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`)
	feed(`{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`)
	feed(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`)

	out, code := runNudge(t, repo, `{"session_id":"s1"}`)
	if code != 0 {
		t.Fatalf("nudge hook exit = %d", code)
	}
	if !strings.Contains(out, "tm_propose") || !strings.Contains(out, "failed_attempt") {
		t.Errorf("expected a propose nudge, got: %q", out)
	}
}

func TestNudgeHookSilentWithNoSignal(t *testing.T) {
	repo := initRepo(t)
	out, code := runNudge(t, repo, `{"session_id":"fresh"}`)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected silence on a fresh session, got: %q", out)
	}
}
