package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestGeminiAfterToolErrorMarksFailed(t *testing.T) {
	a, _ := harness.Get("gemini")
	in := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cargo test"},"tool_response":{"error":"exit status 101"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "cargo test" {
		t.Errorf("event = %+v", ev)
	}
}

func TestGeminiAfterToolSuccess(t *testing.T) {
	a, _ := harness.Get("gemini")
	in := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cargo test"},"tool_response":{"error":""}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success, got %+v", ev)
	}
}

// TestGeminiExitCodeFromLlmContent uses the REAL live shape (2026-06-15): Gemini
// does NOT populate tool_response.error for a failed shell command; it emits an
// "Exit Code: N" line inside tool_response.llmContent (only on a non-zero exit —
// a successful command's llmContent has no such line). The adapter must read
// failure from that line.
func TestGeminiExitCodeFromLlmContent(t *testing.T) {
	a, _ := harness.Get("gemini")
	fail := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"llmContent":"Output: Get-Content: Cannot find path ...\nExit Code: 1\nProcess Group PGID: 3172"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(fail))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "cat tmcheck.txt" {
		t.Errorf("failing result: event = %+v", ev)
	}
	pass := `{"session_id":"s1","tool_name":"run_shell_command","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"llmContent":"Output: ok\nProcess Group PGID: 32380"}}`
	ev, _ = a.Parse(harness.PostTool, strings.NewReader(pass))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("passing result (no Exit Code line) should be a success outcome, got %+v", ev)
	}
}

func TestGeminiRenderBlock(t *testing.T) {
	a, _ := harness.Get("gemini")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	if !strings.Contains(b.String(), `"decision":"deny"`) || !strings.Contains(b.String(), `"reason":"checks"`) {
		t.Errorf("render = %s", b.String())
	}
}

func TestGeminiRenderContext(t *testing.T) {
	a, _ := harness.Get("gemini")
	var b strings.Builder
	a.Render(harness.PostTool, harness.Decision{Context: "constraint"}, &b)
	if !strings.Contains(b.String(), `"additionalContext":"constraint"`) {
		t.Errorf("render = %s", b.String())
	}
}
