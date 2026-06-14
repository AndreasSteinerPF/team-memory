package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCopilotErrorOccurredMarksFailed(t *testing.T) {
	a, _ := harness.Get("copilot")
	// Copilot signals a tool failure with the errorOccurred event (no exit code).
	in := `{"sessionId":"s1","hookEventName":"errorOccurred","toolName":"bash","toolArgs":"{\"command\":\"npm test\"}"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "npm test" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCopilotErrorFieldMarksFailed(t *testing.T) {
	a, _ := harness.Get("copilot")
	in := `{"sessionId":"s1","hookEventName":"postToolUse","toolName":"bash","toolArgs":"{\"command\":\"npm test\"}","error":"exit status 1"}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if !ev.Failed || !ev.HasOutcome {
		t.Errorf("expected failure outcome, got %+v", ev)
	}
}

func TestCopilotExitCodeFallback(t *testing.T) {
	a, _ := harness.Get("copilot")
	in := `{"sessionId":"s1","hookEventName":"postToolUse","toolName":"bash","toolArgs":"{\"command\":\"npm test\"}","toolResult":{"exitCode":2}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if !ev.Failed || !ev.HasOutcome {
		t.Errorf("expected failure via exitCode, got %+v", ev)
	}
}

func TestCopilotParsePostToolSuccess(t *testing.T) {
	a, _ := harness.Get("copilot")
	in := `{"sessionId":"s1","hookEventName":"postToolUse","toolName":"bash","toolArgs":"{\"command\":\"npm test\"}","toolResult":{"exitCode":0}}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

// TestCopilotToolArgsObject confirms toolArgs is also accepted as a bare object,
// and that the legacy snake_case tool_input shape still parses.
func TestCopilotToolArgsVariants(t *testing.T) {
	a, _ := harness.Get("copilot")
	cases := []string{
		`{"sessionId":"s1","hookEventName":"postToolUse","toolName":"bash","toolArgs":{"command":"go build"}}`,
		`{"session_id":"s1","hook_event_name":"postToolUse","tool_name":"bash","tool_input":{"command":"go build"}}`,
	}
	for _, in := range cases {
		ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
		if ev.Command != "go build" || !ev.HasOutcome {
			t.Errorf("input %s → event %+v", in, ev)
		}
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

func TestCopilotRenderAdditionalContext(t *testing.T) {
	a, _ := harness.Get("copilot")
	var b strings.Builder
	a.Render(harness.PostTool, harness.Decision{Context: "fragile area"}, &b)
	if !strings.Contains(b.String(), `"additionalContext":"fragile area"`) {
		t.Errorf("render = %s", b.String())
	}
}
