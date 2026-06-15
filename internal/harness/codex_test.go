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

// Real codex 0.139.0 success payloads carry tool_response as a STRING (the
// command output), not an {exit_code} object. Parse must accept it without
// erroring and treat it as a (passing) command outcome.
func TestCodexParsePostToolStringResponse(t *testing.T) {
	a, _ := harness.Get("codex")
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"echo hello"},"tool_response":"hello\r\n"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatalf("string tool_response must not error: %v", err)
	}
	if !ev.HasOutcome || ev.Failed || ev.Command != "echo hello" {
		t.Errorf("event = %+v (want HasOutcome, !Failed, command=echo hello)", ev)
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
