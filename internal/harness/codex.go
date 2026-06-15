package harness

import (
	"bytes"
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
		// tool_response is polymorphic: a successful shell PostToolUse carries it
		// as a plain string (the command output, e.g. "hello\r\n"), confirmed
		// against live codex 0.139.0, while the published docs describe an object
		// with exit_code. Decode it raw and interpret per-shape so a string payload
		// does not error. NB: in codex 0.139.0 a FAILING tool call emits no
		// PostToolUse at all (only PreToolUse then Stop — see prd.md §10.6), so the
		// exit_code branch only ever fires if a future codex emits a non-zero
		// object; today every PostToolUse we see is a success.
		ToolResponse json.RawMessage `json:"tool_response"`
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
		ev.Failed = codexCommandFailed(raw.ToolResponse)
	}
	return ev, nil
}

// codexCommandFailed reports a non-zero exit only when tool_response is a JSON
// object carrying a non-zero exit_code. A string tool_response (codex's
// success-output shape) or a missing/empty field is treated as not-failed —
// never an error.
func codexCommandFailed(rawResp json.RawMessage) bool {
	t := bytes.TrimSpace(rawResp)
	if len(t) == 0 || t[0] != '{' {
		return false
	}
	var obj struct {
		ExitCode *int `json:"exit_code"`
	}
	if err := json.Unmarshal(t, &obj); err != nil {
		return false
	}
	return obj.ExitCode != nil && *obj.ExitCode != 0
}

// Render mirrors the Claude wire shape; Codex accepts the same hookSpecificOutput
// fields. Codex's PostToolUse payload is snake_case; live codex 0.139.0 reports
// the shell tool as tool_name: "Bash" with a string tool_response (not an
// exit_code object), and file edits report tool_name: "apply_patch" (per OpenAI's
// hook docs). See prd.md §10.6 for the live findings and the failure-sensing
// caveat (failed tool calls emit no PostToolUse).
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
