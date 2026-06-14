package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCopilotParsePostToolUseFailureMarksFailed(t *testing.T) {
	a, _ := harness.Get("copilot")
	// Copilot's postToolUseFailure event marks the fail half (no exit code needed).
	in := `{"session_id":"s1","hook_event_name":"postToolUseFailure","tool_name":"shell","tool_input":{"command":"npm test"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "npm test" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCopilotParsePostToolSuccess(t *testing.T) {
	a, _ := harness.Get("copilot")
	in := `{"session_id":"s1","hook_event_name":"postToolUse","tool_name":"shell","tool_input":{"command":"npm test"},"toolResult":{"exitCode":0}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

func TestCopilotRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("copilot")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	if !strings.Contains(b.String(), `"permissionDecision":"deny"`) {
		t.Errorf("render = %s", b.String())
	}
}
