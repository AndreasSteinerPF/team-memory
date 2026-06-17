package harness_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

// TestClaudeParsePostToolExitCode exercises the forward-compat exit_code path:
// real Claude Code never sends a failure PostToolUse (see
// TestClaudeSuccessPostToolHasNoExitCode), but the adapter still honors an
// exit_code object should a future version deliver one.
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

// TestClaudeSuccessPostToolHasNoExitCode pins the REAL successful-Bash shape
// (CLI 2.1.177, 2026-06-15): tool_response is {stdout,stderr,interrupted,
// isImage,noOutputExpected} with no exit_code. It must parse to a success
// outcome. (Claude fires no PostToolUse at all on a failed command, so this is
// the only command-outcome shape the adapter sees live — hence
// PostToolFailureSensor = no in the capability matrix.)
func TestClaudeSuccessPostToolHasNoExitCode(t *testing.T) {
	a, _ := harness.Get("claude")
	in := `{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"cat tmcheck.txt"},"tool_response":{"stdout":"ok","stderr":"","interrupted":false,"isImage":false,"noOutputExpected":false}}`
	ev, err := a.Parse(harness.PostTool, strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if !ev.HasOutcome || ev.Failed || ev.Command != "cat tmcheck.txt" {
		t.Errorf("real success payload should be a non-failed outcome, got %+v", ev)
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

// TestClaudeRenderStopAdvisoryUsesPlainStdout pins Claude Code's Stop-hook
// output contract: the Stop event rejects `hookSpecificOutput` entirely
// (live-verified 2026-06-16; the envelope is only valid for PreToolUse,
// PostToolUse, UserPromptSubmit, PostToolBatch). The Stop schema accepts
// top-level fields (decision, reason, systemMessage, stopReason, etc.) but
// a nudge wants neither a forced continuation (decision=block) nor a
// user-only systemMessage; plain stdout is the only remaining shape, so
// Render must emit it as plain text (no JSON wrapper) — the test below
// pins that. Note: live dogfooding (2026-06-17) revealed Claude Code does
// NOT actually surface Stop-hook stdout to the agent's next turn, so the
// nudge engine queues the same text in journal.Pending and re-injects via
// UserPromptSubmit additionalContext (see internal/cli/nudge.go +
// internal/cli/signal.go). The plain-text emission here is still required
// for non-Claude harnesses that DO surface Stop stdout.
func TestClaudeRenderStopAdvisoryUsesPlainStdout(t *testing.T) {
	a, _ := harness.Get("claude")
	var b bytes.Buffer
	if err := a.Render(harness.Stop, harness.Decision{Context: "tm: fragile area, consider tm_propose"}, &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "tm: fragile area, consider tm_propose") {
		t.Errorf("render missing advisory text: %q", out)
	}
	if strings.Contains(out, "hookSpecificOutput") {
		t.Errorf("Stop output must not include hookSpecificOutput (Claude Code's Stop schema rejects it): %q", out)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("Stop advisory must be plain text, not JSON: %q", out)
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
