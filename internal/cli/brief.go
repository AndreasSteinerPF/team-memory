package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

// newBriefCmd emits the session-start briefing: live ledger counts plus the
// standing instructions for the voluntary verbs. Designed to run as a
// SessionStart hook in Claude Code (plain text), Codex CLI and Continue CLI
// (plain text / Claude-compatible), and via JSON envelopes for Copilot CLI,
// Cursor, and Gemini CLI.
func newBriefCmd(g *globalOpts) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "Emit a session-start briefing for agent hooks (live counts + usage instructions)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				// A session hook must never break or spam a session, so we
				// deliberately swallow EVERY openEnv failure (not just the
				// expected "no ledger yet" case) and succeed silently. A
				// genuinely broken setup — corrupt index, git error — still
				// surfaces loudly through every other command (tm list/status/
				// sync all return this error), so nothing is hidden; only the
				// hook stays quiet.
				return nil
			}
			defer e.close()
			text, err := buildBrief(e)
			if err != nil {
				return nil // same hook-safety rule: degrade to silence
			}
			out := cmd.OutOrStdout()
			switch format {
			case "text", "claude", "codex", "continue":
				_, werr := fmt.Fprint(out, text)
				return werr
			case "copilot":
				return json.NewEncoder(out).Encode(map[string]string{"additionalContext": text})
			case "cursor":
				return json.NewEncoder(out).Encode(map[string]string{"additional_context": text})
			case "gemini":
				return json.NewEncoder(out).Encode(map[string]any{
					"hookSpecificOutput": map[string]string{
						"hookEventName":     "SessionStart",
						"additionalContext": text,
					},
				})
			default:
				return fmt.Errorf("unknown --format %q (want text|claude|codex|continue|copilot|cursor|gemini)", format)
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "text | claude | codex | continue | copilot | cursor | gemini")
	return cmd
}

// buildBrief renders the briefing text: one line of live counts, then the
// standing instructions (prd.md §10.1). Kept short on purpose — this lands in
// every session's context.
func buildBrief(e *env) (string, error) {
	rows, err := e.idx.All()
	if err != nil {
		return "", err
	}
	counts := map[model.Status]int{}
	for _, m := range rows {
		counts[m.Status]++
	}
	// active/provisional/contested are the actionable statuses at session
	// start; stale and rejected are intentionally omitted from the count line.
	var b strings.Builder
	fmt.Fprintf(&b, "TeamMemory: shared project memory is active in this repo — %d active, %d provisional, %d contested memories.\n",
		counts[model.StatusActive], counts[model.StatusProvisional], counts[model.StatusContested])
	b.WriteString("- Relevant memories are injected automatically when you edit matching files (PreToolUse hook). For multi-file planning — or if this agent has no TeamMemory edit hook — call the tm_check_action MCP tool with the target paths first.\n")
	b.WriteString("- Record durable project judgment with tm_propose when you discover a non-obvious failure, a hidden constraint, a fragile area, a stale doc, or an undocumented decision. Never record session state, trivia, facts derivable from the code, or system/OS-specific details (per-OS flags, interpreter names, local toolchain versions) — memories are shared across the whole team.\n")
	b.WriteString("- When your work bears on a memory you were shown, react with tm_observe: confirm with evidence, contradict with evidence, adjust_scope, or mark_stale.\n")
	if counts[model.StatusProvisional] > 0 {
		b.WriteString("- Provisional memories await independent validation; if your work touches their scope, your confirmation or contradiction matters.\n")
	}
	return b.String(), nil
}
