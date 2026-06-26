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

// TestCopilotExitCodeFromResultText uses the REAL live shape (copilot 1.0.62,
// 2026-06-15): a shell tool carries no structured exitCode, reports
// resultType:"success" even on a failed command, and embeds the true exit code
// in toolResult.textResultForLlm as "<shellId: N completed with exit code C>".
// The adapter must read failure from that trailing exit code.
func TestCopilotExitCodeFromResultText(t *testing.T) {
	a, _ := harness.Get("copilot")
	fail := `{"sessionId":"s1","toolName":"powershell","toolArgs":"{\"command\":\"go test ./...\"}","toolResult":{"resultType":"success","textResultForLlm":"FAIL\n<shellId: 0 completed with exit code 1>"}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(fail))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "go test ./..." {
		t.Errorf("failing result: event = %+v", ev)
	}
	pass := `{"sessionId":"s1","toolName":"powershell","toolArgs":"{\"command\":\"go test ./...\"}","toolResult":{"resultType":"success","textResultForLlm":"ok\n<shellId: 1 completed with exit code 0>"}}`
	ev, _ = a.Parse(harness.PostTool, strings.NewReader(pass))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("passing result should be a success outcome, got %+v", ev)
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

func TestCopilotParseFilePathWithSpaces(t *testing.T) {
	a, _ := harness.Get("copilot")
	cases := []string{
		`{"sessionId":"s1","hookEventName":"postToolUse","toolName":"edit","toolArgs":"{\"file_path\":\"docs/space dir/design note.md\"}"}`,
		`{"sessionId":"s1","hookEventName":"postToolUse","toolName":"edit","toolArgs":{"path":"docs/space dir/design note.md"}}`,
	}
	for _, in := range cases {
		ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		if ev.FilePath != "docs/space dir/design note.md" {
			t.Errorf("input %s -> FilePath = %q", in, ev.FilePath)
		}
		if ev.Command != "" || ev.HasOutcome {
			t.Errorf("file edit must not be classified as a command outcome: %+v", ev)
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
