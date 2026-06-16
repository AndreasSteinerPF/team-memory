package harness

import (
	"encoding/json"
	"fmt"
	"io"
)

func init() { register(claude{}) }

type claude struct{}

func (claude) Name() string { return "claude" }

// Parse reads a Claude Code hook payload. NOTE (verified live, CLI 2.1.177,
// 2026-06-15; see prd.md §10.6): Claude Code fires PostToolUse only after a tool
// completes *successfully*, so a failing Bash command emits PreToolUse then no
// PostToolUse at all — command-failure sensing cannot fire (the capability
// matrix sets claude PostToolFailureSensor = no, like codex). A *successful*
// Bash tool_response is {stdout,stderr,interrupted,isImage,noOutputExpected}
// with no exit_code. The exit_code check below therefore never trips on real
// claude payloads; it is retained for forward-compat should a future version
// deliver a failure PostToolUse with an exit code.
func (claude) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"tool_input"`
		ToolResponse struct {
			ExitCode *int `json:"exit_code"`
		} `json:"tool_response"`
	}
	if err := decodeJSON(r, &raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind:      kind,
		SessionID: raw.SessionID,
		ToolName:  raw.ToolName,
		Command:   raw.ToolInput.Command,
		FilePath:  raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.ToolResponse.ExitCode != nil && *raw.ToolResponse.ExitCode != 0
	}
	return ev, nil
}

// Render emits the hook decision. Stop is special: Claude Code's Stop hook
// schema (live-verified 2026-06-16) rejects `hookSpecificOutput` entirely —
// that envelope is only valid for PreToolUse / PostToolUse /
// UserPromptSubmit / PostToolBatch. An advisory Decision on Stop must render
// as plain text to stdout, which Claude Code surfaces directly (matches the
// README's "never a forced turn" promise for the nudge engine). A block
// Decision on Stop — which the nudge engine never produces, but we render
// defensively — uses the top-level `decision`/`reason` fields the Stop
// schema does accept.
func (claude) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil // emit nothing; the action proceeds
	}
	if kind == Stop {
		if d.Block {
			return json.NewEncoder(w).Encode(struct {
				Decision string `json:"decision"`
				Reason   string `json:"reason"`
			}{"block", d.Reason})
		}
		_, err := fmt.Fprintln(w, d.Context)
		return err
	}
	type spec struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
	}
	s := spec{HookEventName: eventName(kind)}
	if d.Block {
		s.PermissionDecision = "deny"
		s.PermissionDecisionReason = d.Reason
	} else {
		s.AdditionalContext = d.Context
	}
	return json.NewEncoder(w).Encode(struct {
		HookSpecificOutput spec `json:"hookSpecificOutput"`
	}{s})
}

func eventName(kind EventKind) string {
	switch kind {
	case PreTool:
		return "PreToolUse"
	case PostTool:
		return "PostToolUse"
	case Stop:
		return "Stop"
	case PromptSubmit:
		return "UserPromptSubmit"
	}
	return ""
}
