package harness

import (
	"encoding/json"
	"io"
)

func init() { register(copilot{}) }

type copilot struct{}

func (copilot) Name() string { return "copilot" }

func (copilot) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
		ToolName      string `json:"tool_name"`
		ToolInput     struct {
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"tool_input"`
		ToolResult struct {
			ExitCode *int `json:"exitCode"`
		} `json:"toolResult"`
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
		switch {
		case raw.HookEventName == "postToolUseFailure":
			ev.Failed = true
		case raw.ToolResult.ExitCode != nil:
			ev.Failed = *raw.ToolResult.ExitCode != 0
		}
	}
	return ev, nil
}

// Render emits Copilot's hook decision. VERIFY (spec §10): confirm a script
// (non-SDK) postToolUse hook actually receives additionalContext on output and
// exitCode/postToolUseFailure on input; if the script path drops
// additionalContext, packaging must ship the SDK hook variant. Adjust only here.
func (copilot) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		AdditionalContext string `json:"additionalContext"`
	}{d.Context})
}
