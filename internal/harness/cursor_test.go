package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCursorFailureEventMarksFailed(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"session_id":"s1","hook_event_name":"postToolUseFailure","command":"pytest"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "pytest" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCursorShellCompletionIsSuccess(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"session_id":"s1","hook_event_name":"afterShellExecution","command":"pytest","output":"..."}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

func TestCursorRenderBlockUsesAgentMessage(t *testing.T) {
	a, _ := harness.Get("cursor")
	var b strings.Builder
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "checks"}, &b)
	out := b.String()
	if !strings.Contains(out, `"permission":"deny"`) || !strings.Contains(out, `"agent_message":"checks"`) {
		t.Errorf("render = %s", out)
	}
}

func TestCursorRenderContextUsesAdditionalContext(t *testing.T) {
	a, _ := harness.Get("cursor")
	var b strings.Builder
	a.Render(harness.PostTool, harness.Decision{Context: "fragile area"}, &b)
	if !strings.Contains(b.String(), `"additional_context":"fragile area"`) {
		t.Errorf("render = %s", b.String())
	}
}
