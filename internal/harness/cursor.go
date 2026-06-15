package harness

import (
	"encoding/json"
	"io"
)

// Cursor adapter (prd.md §10.6 / spec §6.4): derives command pass/fail from a
// failure flag (the postToolUseFailure event, discriminated by hook_event_name)
// rather than a numeric exit code.
//
// Why no special double-event dedup is needed (Cursor only): on a failing
// command Cursor fires both afterShellExecution (Failed=false) and
// postToolUseFailure (Failed=true) — two signal --hook invocations, so two
// journal CmdOutcomes at consecutive turns. This is safe because slice 1's
// detectFailPass pairs a Failed=true outcome with a later same-signature
// Failed=false outcome ONLY IF editBetween(j, fail.Turn, pass.Turn) is true.
// The two synthetic outcomes of a single command have no edit between their
// turns, so no spurious recovery fires. A genuine recovery (fail → edit →
// re-run) still satisfies the predicate. Keep the bounded [fail.Turn,pass.Turn]
// check; do NOT add dedup here.
//
// Confirmed against real cursor-agent payloads (cursor 2026.06.12):
//   - afterShellExecution: top-level "command" + "output", NO exit code; fires for
//     both passing and failing commands, so it alone cannot signal failure.
//   - postToolUseFailure: the failure signal. For a failed SHELL command it carries
//     tool_name "Shell" with the command nested at tool_input.command (NOT
//     top-level) plus error_message/failure_type. For a non-shell tool (e.g. a
//     failed Read) it carries tool_input.file_path and no command.
//   - afterFileEdit: top-level "file_path" + edits[].
// So command is read from top-level OR tool_input.command; file_path only from
// top-level (afterFileEdit) — pulling tool_input.file_path would make a failed
// Read look like an edit and record a spurious churn signal.
//
// VERIFY (prd.md §10.6): the headless cursor-agent CLI does NOT fire the "stop" or
// "beforeSubmitPrompt" hooks, so the nudge (Stop) and prompt signals are only
// exercised in the IDE / via replayed fixtures, not by live capture.
func init() { register(cursor{}) }

type cursor struct{}

func (cursor) Name() string { return "cursor" }

func (cursor) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
		Command       string `json:"command"`
		FilePath      string `json:"file_path"`
		ToolInput     struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := decodeJSON(r, &raw); err != nil {
		return Event{}, err
	}
	// postToolUseFailure nests a failed shell command under tool_input.command;
	// every other command-bearing event uses the top-level field.
	command := raw.Command
	if command == "" {
		command = raw.ToolInput.Command
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID,
		Command: command, FilePath: raw.FilePath,
	}
	// Only shell events carry a command outcome. Guarding on hook_event_name
	// stops a file-edit event (afterFileEdit) that happens to carry a command
	// field from being misrecorded as a command outcome. A non-shell
	// postToolUseFailure (e.g. a failed Read) has no command and is ignored here.
	if kind == PostTool && command != "" &&
		(raw.HookEventName == "afterShellExecution" || raw.HookEventName == "postToolUseFailure") {
		ev.HasOutcome = true
		ev.Failed = raw.HookEventName == "postToolUseFailure"
	}
	return ev, nil
}

// Render emits the Cursor hook decision. VERIFY (prd.md §10.6; docs/verification/cross-harness.md):
// confirm permission/agent_message shape for block and additional_context shape for allow;
// adjust only here if a live payload differs.
func (cursor) Render(kind EventKind, d Decision, w io.Writer) error {
	if d.Empty() {
		return nil
	}
	if d.Block {
		return json.NewEncoder(w).Encode(struct {
			Permission   string `json:"permission"`
			AgentMessage string `json:"agent_message"`
		}{"deny", d.Reason})
	}
	return json.NewEncoder(w).Encode(struct {
		AdditionalContext string `json:"additional_context"`
	}{d.Context})
}
