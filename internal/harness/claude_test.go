package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

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
