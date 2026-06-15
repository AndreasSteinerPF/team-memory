package harness

import (
	"encoding/json"
	"io"
	"regexp"
	"strconv"
)

func init() { register(copilot{}) }

type copilot struct{}

func (copilot) Name() string { return "copilot" }

// copilotExitCodeRe matches the exit code Copilot embeds in a shell tool result's
// human-readable text, e.g. "...<shellId: 0 completed with exit code 1>". Live
// capture (copilot 1.0.62, 2026-06-15) showed Copilot does NOT emit a structured
// toolResult.exitCode for shell commands: the result text carries the exit code,
// and toolResult.resultType is "success" even when the command exited non-zero
// (it reports that the TOOL ran, not that the command passed).
var copilotExitCodeRe = regexp.MustCompile(`exit code (\d+)`)

// Parse reads a Copilot CLI hook payload. Copilot uses camelCase fields
// (toolName, toolArgs) and delivers tool arguments as a JSON-encoded string;
// alternate/older shapes used snake_case tool_input, so both are accepted. A
// tool failure surfaces as the errorOccurred event, a non-empty error field, a
// non-zero toolResult.exitCode (forward-compat), or — for shell commands, the
// live shape — a non-zero "exit code N" parsed from toolResult.textResultForLlm.
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
			ExitCode         *int   `json:"exitCode"`
			TextResultForLlm string `json:"textResultForLlm"`
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
		default:
			if code, ok := parseTrailingExitCode(copilotExitCodeRe, raw.ToolResult.TextResultForLlm); ok {
				ev.Failed = code != 0
			}
		}
	}
	return ev, nil
}

// parseTrailingExitCode returns the exit code captured by re from text, using
// the LAST match (a shell result may mention "exit code" in its output before
// the trailing "<shellId: N completed with exit code C>" marker). ok is false
// when text carries no exit-code marker.
func parseTrailingExitCode(re *regexp.Regexp, text string) (int, bool) {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	code, err := strconv.Atoi(last[1])
	if err != nil {
		return 0, false
	}
	return code, true
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
