package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func TestCodexParsePostToolExitCode(t *testing.T) {
	a, _ := harness.Get("codex")
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"go build"},"tool_response":{"exit_code":2}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "go build" {
		t.Errorf("event = %+v", ev)
	}
}

func TestCodexRenderPreToolBlock(t *testing.T) {
	a, _ := harness.Get("codex")
	var b bytes.Buffer
	a.Render(harness.PreTool, harness.Decision{Block: true, Reason: "run checks"}, &b)
	out := b.String()
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "run checks") {
		t.Errorf("render = %s", out)
	}
}

func TestCodexRenderPostToolContext(t *testing.T) {
	a, _ := harness.Get("codex")
	var b bytes.Buffer
	a.Render(harness.PostTool, harness.Decision{Context: "known constraint"}, &b)
	if !strings.Contains(b.String(), `"additionalContext":"known constraint"`) {
		t.Errorf("render = %s", b.String())
	}
}
