package harness

import (
	"encoding/json"
	"io"
)

func init() { register(codex{}) }

type codex struct{}

func (codex) Name() string { return "codex" }

func (codex) Parse(kind EventKind, r io.Reader) (Event, error) {
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
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID, ToolName: raw.ToolName,
		Command: raw.ToolInput.Command, FilePath: raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.ToolResponse.ExitCode != nil && *raw.ToolResponse.ExitCode != 0
	}
	return ev, nil
}

// Render mirrors the Claude wire shape; Codex accepts the same hookSpecificOutput
// fields. Codex's PostToolUse payload is snake_case with the exit code at
// tool_response.exit_code, and file edits report tool_name: "apply_patch" —
// both confirmed against OpenAI's published hook docs (prd.md §10.6). The
// harness test suite's gated live smoke test is the remaining belt-and-braces check.
func (codex) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
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
