package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/cli"
)

func runSignal(t *testing.T, repo, stdin string) int {
	t.Helper()
	var out, errb bytes.Buffer
	return cli.Run([]string{"--repo", repo, "signal", "--hook"}, strings.NewReader(stdin), &out, &errb)
}

func TestSignalHookRecordsCommandOutcome(t *testing.T) {
	repo := initRepo(t)
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":1}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"internal/index/x.go"}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
	if code := runSignal(t, repo, `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."},"tool_response":{"exit_code":0}}`); code != 0 {
		t.Fatalf("signal hook exit = %d", code)
	}
}
