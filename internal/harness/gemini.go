package harness

import (
	"encoding/json"
	"io"
)

// Gemini adapter (prd.md §10.6 / spec §6.5): derives command pass/fail from
// AfterTool's tool_response.error field — a non-empty string signals failure.
// Unlike Cursor's dual-event model, Gemini fires a single AfterTool per
// command (error set or empty), so no double-event reasoning is required.
//
// VERIFY (prd.md §10.6): confirm against the pinned Gemini release tag (schema
// differs from main); confirm AfterTool.additionalContext is model-visible
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
			Error string `json:"error"`
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
		ev.Failed = raw.ToolResponse.Error != ""
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
