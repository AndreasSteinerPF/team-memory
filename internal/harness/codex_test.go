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

// TestCodexParseApplyPatchExtractsPath uses the REAL apply_patch payload (codex
// gpt-5.5, 2026-06-16): the edited path lives in the patch text at
// tool_input.command ("*** Add File: <path>"), not a file_path field. The adapter
// must surface it as FilePath (an EDIT) so path-scoped requirements/advisories
// match — and must NOT record it as a command (which broke codex file blocking).
func TestCodexParseApplyPatchExtractsPath(t *testing.T) {
	a, _ := harness.Get("codex")
	in := `{"session_id":"s1","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Add File: billing/migrations/m.sql\n+-- v1\n*** End Patch\n"}}`
	ev, err := a.Parse(harness.PreTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if ev.FilePath != "billing/migrations/m.sql" {
		t.Errorf("FilePath = %q, want billing/migrations/m.sql", ev.FilePath)
	}
	if ev.Command != "" || ev.HasOutcome {
		t.Errorf("apply_patch must be an edit, not a command outcome: %+v", ev)
	}
}

// TestCodexParseApplyPatchUpdateAndDelete covers the Update/Delete File headers.
func TestCodexParseApplyPatchUpdateAndDelete(t *testing.T) {
	a, _ := harness.Get("codex")
	for _, tc := range []struct{ in, want string }{
		{`{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: internal/index/x.go\n*** End Patch\n"}}`, "internal/index/x.go"},
		{`{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Delete File: old/legacy.go\n*** End Patch\n"}}`, "old/legacy.go"},
	} {
		ev, _ := a.Parse(harness.PreTool, strings.NewReader(tc.in))
		if ev.FilePath != tc.want {
			t.Errorf("FilePath = %q, want %q", ev.FilePath, tc.want)
		}
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

func TestCodexRenderStopAdvisoryWritesNothing(t *testing.T) {
	a, _ := harness.Get("codex")
	var b bytes.Buffer
	if err := a.Render(harness.Stop, harness.Decision{Context: "tm: consider tm_propose"}, &b); err != nil {
		t.Fatal(err)
	}
	if b.Len() != 0 {
		t.Fatalf("Stop advisory must not write unsupported Codex Stop output: %q", b.String())
	}
}
