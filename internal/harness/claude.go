package harness

import (
	"encoding/json"
	"io"
)

func init() { register(claude{}) }

type claude struct{}

func (claude) Name() string { return "claude" }

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
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
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

// Render emits the hook decision. VERIFY (spec §10): on Stop the
// context-injection shape may differ across Claude Code versions — some surface
// Stop stdout directly, others require {"decision":"block","reason":...} (which
// forces a turn, undesirable for a low-pressure nudge). This Render is the one
// place that decides it; adjust here if a live payload differs.
func (claude) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil // emit nothing; the action proceeds
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
