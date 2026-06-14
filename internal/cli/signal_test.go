package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

func TestSignalHookInjectsAdvisoryForEditedPath(t *testing.T) {
	repo := initRepo(t)
	// Propose an active, low-risk decision scoped to docs/** (activates immediately).
	var o, e bytes.Buffer
	cli.Run([]string{"--repo", repo, "propose", "decision", "--title", "Doc style", "--scope", "docs/**", "--guidance", "Use sentence case"}, strings.NewReader(""), &o, &e)

	// A non-Claude harness edit to docs/x.md should surface the memory as context.
	in := `{"session_id":"s1","tool_name":"Edit","tool_input":{"file_path":"docs/x.md"}}`
	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "signal", "--hook", "--harness", "codex"}, strings.NewReader(in), &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Doc style") {
		t.Errorf("expected advisory injection for docs/x.md, got: %q", out.String())
	}
}

func TestSignalHookInjectsAdvisoryForAbsolutePath(t *testing.T) {
	repo := initRepo(t)
	var o, e bytes.Buffer
	cli.Run([]string{"--repo", repo, "propose", "decision", "--title", "Doc style", "--scope", "docs/**", "--guidance", "Use sentence case"}, strings.NewReader(""), &o, &e)

	// An absolute in-repo path is what Claude Code / Codex actually send; this
	// exercises relPath's production branch (Abs+Rel yields a clean repo-relative
	// path; the ".." fallback does NOT trigger).
	abs := filepath.Join(repo, "docs", "y.md")
	payload := struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
	}{SessionID: "s1", ToolName: "Edit"}
	payload.ToolInput.FilePath = abs
	in, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := cli.Run([]string{"--repo", repo, "signal", "--hook", "--harness", "codex"}, bytes.NewReader(in), &out, &errb)
	if code != 0 {
		t.Fatalf("exit = %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Doc style") {
		t.Errorf("expected advisory injection for absolute docs path, got: %q", out.String())
	}
}
