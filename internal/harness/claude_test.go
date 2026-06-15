package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// TestClaudeParsePostToolExitCode exercises the forward-compat exit_code path:
// real Claude Code never sends a failure PostToolUse (see
// TestClaudeSuccessPostToolHasNoExitCode), but the adapter still honors an
// exit_code object should a future version deliver one.
func TestClaudeParsePostToolExitCode(t *testing.T) {
	a, err := harness.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test"},"tool_response":{"exit_code":1}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if ev.SessionID != "s1" || ev.Command != "go test" || !ev.Failed || !ev.HasOutcome {
		t.Errorf("parsed event wrong: %+v", ev)
	}
}

// TestClaudeSuccessPostToolHasNoExitCode pins the REAL successful-Bash shape
// (CLI 2.1.177, 2026-06-15): tool_response is {stdout,stderr,interrupted,
// isImage,noOutputExpected} with no exit_code. It must parse to a success
// outcome. (Claude fires no PostToolUse at all on a failed command, so this is
// the only command-outcome shape the adapter sees live — hence
// PostToolFailureSensor = no in the capability matrix.)
func TestClaudeSuccessPostToolHasNoExitCode(t *testing.T) {
	a, _ := harness.Get("claude")
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"stdout":"ok","stderr":"","interrupted":false,"isImage":false,"noOutputExpected":false}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.HasOutcome || ev.Failed || ev.Command != "cat tmcheck.txt" {
		t.Errorf("real success payload should be a non-failed outcome, got %+v", ev)
	}
}

func TestClaudeRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("claude")
	var b bytes.Buffer
	if err := a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "do the checks"}, &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "do the checks") {
		t.Errorf("render missing deny decision: %s", out)
	}
}

func TestClaudeRenderEmptyDecisionWritesNothing(t *testing.T) {
	a, _ := harness.Get("claude")
	var b bytes.Buffer
	if err := a.Render(harness.PostTool, harness.Decision{}, &b); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != "" {
		t.Errorf("empty decision should write nothing, got: %q", b.String())
	}
}

func TestUnknownHarnessErrors(t *testing.T) {
	if _, err := harness.Get("nope"); err == nil {
		t.Error("expected error for unknown harness")
	}
}
