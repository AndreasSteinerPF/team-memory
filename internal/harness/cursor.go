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
// VERIFY (prd.md §10.6): confirm afterShellExecution/postToolUseFailure payload
// field names (command, hook_event_name) and that additional_context injects
// model-visible text on allow.
func init() { register(cursor{}) }

type cursor struct{}

func (cursor) Name() string { return "cursor" }

func (cursor) Parse(kind EventKind, r io.Reader) (Event, error) {
	var raw struct {
		SessionID     string `json:"session_id"`
		HookEventName string `json:"hook_event_name"`
		Command       string `json:"command"`
		FilePath      string `json:"file_path"`
	}
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Event{}, err
	}
	ev := Event{
		Kind: kind, SessionID: raw.SessionID,
		Command: raw.Command, FilePath: raw.FilePath,
	}
	// Only shell events carry a command outcome. Guarding on hook_event_name
	// stops a file-edit event (afterFileEdit) that happens to carry a command
	// field from being misrecorded as a command outcome.
	if kind == PostTool && raw.Command != "" &&
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
