package harness

import (
	"encoding/json"
	"io"
	"regexp"
)

// geminiExitCodeRe matches the exit code Gemini embeds in a shell tool's
// llmContent, e.g. "Output: ...\nExit Code: 1\nProcess Group PGID: 3172". Live
// capture (2026-06-15) showed Gemini does NOT populate tool_response.error for a
// failed shell command: the "Exit Code: N" line appears only on a non-zero exit
// (a successful command's llmContent has no such line), so its presence with a
// non-zero value is the failure signal.
var geminiExitCodeRe = regexp.MustCompile(`Exit Code:\s*(\d+)`)

// Gemini adapter (prd.md §10.6 / spec §6.5): derives command pass/fail from
// AfterTool's tool_response. Unlike Cursor's dual-event model, Gemini fires a
// single AfterTool per command (it DOES fire on failure, unlike Claude/Codex),
// so no double-event reasoning is required. Failure is read from a non-zero
// "Exit Code: N" line in tool_response.llmContent (live shape) or a non-empty
// tool_response.error (forward-compat / older shape).
//
// VERIFY (prd.md §10.6): confirm AfterTool.additionalContext is model-visible
// (systemMessage is user-only and must NOT be used for advisory injection).
func init() { register(gemini{}) }

type gemini struct{}

func (gemini) Name() string { return "gemini" }

func (gemini) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID string `json:"session_id"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			Command  string `json:"command"`
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
		ToolResponse struct {
			Error      string `json:"error"`
			LlmContent string `json:"llmContent"`
		} `json:"tool_response"`
	}
	if err := decodeJSON(r, &raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID, ToolName: raw.ToolName,
		Command: raw.ToolInput.Command, FilePath: raw.ToolInput.FilePath,
	}
	if kind == PostTool && raw.ToolInput.Command != "" {
		ev.HasOutcome = true
		ev.Failed = raw.ToolResponse.Error != ""
		if !ev.Failed {
			if code, ok := parseTrailingExitCode(geminiExitCodeRe, raw.ToolResponse.LlmContent); ok {
				ev.Failed = code != 0
			}
		}
	}
	return ev, nil
}

// Render emits the Gemini hook decision. VERIFY (prd.md §10.6; docs/verification/cross-harness.md):
// confirm decision/reason shape for block and additionalContext shape for allow;
// adjust only here if a live payload differs.
func (gemini) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}{struct {
		AdditionalContext string `json:"additionalContext"`
	}{d.Context}})
}
