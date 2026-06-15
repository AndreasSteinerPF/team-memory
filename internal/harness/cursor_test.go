package harness_test

import (
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// TestCursorParseToleratesBOM: cursor-agent on Windows prepends a UTF-8 BOM to
// hook stdin (observed live, 2026.06.12). Go's json decoder rejects a leading
// BOM, which silently broke every cursor hook (signal/nudge/block) until the
// shared decodeJSON helper strips it. Parse must accept the BOM-prefixed payload.
func TestCursorParseToleratesBOM(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := "\xef\xbb\xbf" + `{"session_id":"s1","command":"echo hello","hook_event_name":"afterShellExecution"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatalf("BOM-prefixed payload must parse, got: %v", err)
	}
	if ev.SessionID != "s1" || !ev.HasOutcome || ev.Command != "echo hello" {
		t.Errorf("event = %+v (want SessionID=s1, HasOutcome, command=echo hello)", ev)
	}
}

// TestCursorShellFailureMarksFailed uses the REAL postToolUseFailure payload
// captured from cursor-agent for a failing shell command (`cmd /c exit 3`): the
// command is nested under tool_input.command (NOT top-level), the event carries
// tool_name "Shell" + error_message. The adapter must read the nested command.
func TestCursorShellFailureMarksFailed(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"tool_name":"Shell","tool_input":{"command":"cmd /c exit 3","cwd":"","timeout":30000},"error_message":"Command failed with exit code 3","failure_type":"error","session_id":"s1","hook_event_name":"postToolUseFailure"}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Failed || !ev.HasOutcome || ev.Command != "cmd /c exit 3" {
		t.Errorf("event = %+v", ev)
	}
}

// TestCursorShellCompletionIsSuccess uses the REAL afterShellExecution payload:
// top-level command, an output field, and NO exit code (cursor does not report
// one), so a completed shell execution is a success outcome.
func TestCursorShellCompletionIsSuccess(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"command":"echo hello","output":"hello\r\n","duration":1775.9,"session_id":"s1","hook_event_name":"afterShellExecution"}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.Failed || !ev.HasOutcome || ev.Command != "echo hello" {
		t.Errorf("expected success outcome, got %+v", ev)
	}
}

// TestCursorNonShellFailureNotACommandOutcome uses the REAL postToolUseFailure
// payload for a failed Read (file not found): it carries tool_input.file_path but
// no command. It must NOT be recorded as a command outcome, and must NOT set
// FilePath (which would make signal.go record a spurious edit).
func TestCursorNonShellFailureNotACommandOutcome(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"tool_name":"Read","tool_input":{"file_path":"C:\\repo\\note.txt"},"error_message":"File not found","failure_type":"error","session_id":"s1","hook_event_name":"postToolUseFailure"}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.HasOutcome || ev.Command != "" || ev.FilePath != "" {
		t.Errorf("read failure must not be a command outcome or edit; got %+v", ev)
	}
}

// TestCursorFileEditSetsPath uses the REAL afterFileEdit payload: top-level
// file_path + edits[]. It is an edit (FilePath set), not a command outcome.
func TestCursorFileEditSetsPath(t *testing.T) {
	a, _ := harness.Get("cursor")
	in := `{"file_path":"C:\\repo\\note.txt","edits":[{"old_string":"","new_string":"hi"}],"session_id":"s1","hook_event_name":"afterFileEdit"}`
	ev, _ := a.Parse(harness.PostTool, strings.NewReader(in))
	if ev.FilePath != `C:\repo\note.txt` || ev.HasOutcome || ev.Command != "" {
		t.Errorf("expected an edit, got %+v", ev)
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
