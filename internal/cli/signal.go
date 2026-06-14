package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// postHookInput is the PostToolUse event contract (Claude Code). tool_response
// carries the exit code for Bash; nil exit_code ⇒ treat as success.
type postHookInput struct {
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

func newSignalCmd(g *globalOpts) *cobra.Command {
	var hook bool
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Record nudge signals from a PostToolUse event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("signal currently supports only --hook mode")
			}
			var in postHookInput
			if err := json.NewDecoder(cmd.InOrStdin()).Decode(&in); err != nil {
				return fmt.Errorf("signal hook: decode stdin: %w", err)
			}
			if in.SessionID == "" {
				return nil // cannot key a journal without a session
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			store, err := e.nudgeStore()
			if err != nil {
				return err
			}
			j, err := store.Load(in.SessionID)
			if err != nil {
				return err
			}
			j.Turn++ // each PostToolUse advances the within-session clock

			switch {
			case in.ToolInput.Command != "":
				failed := in.ToolResponse.ExitCode != nil && *in.ToolResponse.ExitCode != 0
				j.RecordCommand(in.ToolInput.Command, failed)
			case in.ToolInput.FilePath != "":
				j.RecordEdit(relPath(e, in.ToolInput.FilePath))
			}
			return store.Save(j)
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a PostToolUse event on stdin and record signals")
	return cmd
}

// relPath converts an absolute or repo-relative path to a forward-slash repo path.
func relPath(e *env, p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		if r, err := filepath.Rel(e.repoDir, abs); err == nil {
			return filepath.ToSlash(r)
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}
