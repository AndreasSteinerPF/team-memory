package harness

import (
	"encoding/json"
	"io"
)

func init() { register(copilot{}) }

type copilot struct{}

func (copilot) Name() string { return "copilot" }

// Parse reads a Copilot CLI hook payload. Copilot uses camelCase fields
// (toolName, toolArgs) and delivers tool arguments as a JSON-encoded string;
// alternate/older shapes used snake_case tool_input, so both are accepted. A
// tool failure surfaces as the errorOccurred event, a non-empty error field, or
// a non-zero toolResult.exitCode.
//
// VERIFY (prd.md §10.6; docs/verification/cross-harness.md): the exact failure
// representation and field casing are pinned by the harness test suite's live
// capture. This parser deliberately accepts the documented variants until then.
func (copilot) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionIDSnake string `json:"session_id"`
		SessionID      string `json:"sessionId"`
		EventSnake     string `json:"hook_event_name"`
		Event          string `json:"hookEventName"`
		ToolNameSnake  string `json:"tool_name"`
		ToolName       string `json:"toolName"`
		ToolInput      struct {
			FilePath string `json:"file_path"`
			Command  string `json:"command"`
		} `json:"tool_input"`
		ToolArgs   json.RawMessage `json:"toolArgs"`
		ToolResult struct {
			ExitCode *int `json:"exitCode"`
		} `json:"toolResult"`
		Error string `json:"error"`
	}
	if err := decodeJSON(r, &raw); err != nil {
		return Event{}, err
	}
	command, filePath := raw.ToolInput.Command, raw.ToolInput.FilePath
	if command == "" && filePath == "" && len(raw.ToolArgs) > 0 {
		command, filePath = parseCopilotToolArgs(raw.ToolArgs)
	}
	event := firstNonEmpty(raw.Event, raw.EventSnake)
	ev := Event{
		Kind:      kind,
		SessionID: firstNonEmpty(raw.SessionID, raw.SessionIDSnake),
		ToolName:  firstNonEmpty(raw.ToolName, raw.ToolNameSnake),
		Command:   command,
		FilePath:  filePath,
	}
	if kind == PostTool && command != "" {
		ev.HasOutcome = true
		switch {
		case event == "errorOccurred" || raw.Error != "":
			ev.Failed = true
		case raw.ToolResult.ExitCode != nil:
			ev.Failed = *raw.ToolResult.ExitCode != 0
		}
	}
	return ev, nil
}

// parseCopilotToolArgs extracts the command / file path from Copilot's toolArgs,
// which arrives as a JSON-encoded string (e.g. "{\"command\":\"ls\"}") or, on
// some versions, a bare JSON object.
func parseCopilotToolArgs(raw json.RawMessage) (command, filePath string) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		raw = json.RawMessage(s) // unwrap one JSON-string level
	}
	var args struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	}
	_ = json.Unmarshal(raw, &args)
	return args.Command, firstNonEmpty(args.FilePath, args.Path)
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Render emits Copilot's hook decision as bare camelCase fields (permissionDecision
// on deny, additionalContext on allow), matching Copilot's documented hook-output
// schema. VERIFY (prd.md §10.6; docs/verification/cross-harness.md): the harness
// test suite confirms a script (non-SDK) postToolUse hook's additionalContext is
// model-visible; if a version drops it, ship the SDK hook variant — adjust only here.
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
