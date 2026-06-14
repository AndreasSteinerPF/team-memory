package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
)

func newSignalCmd(g *globalOpts) *cobra.Command {
	var hook bool
	var harnessName string
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Record nudge signals from a PostToolUse event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("signal currently supports only --hook mode")
			}
			a, err := harness.Get(harnessName)
			if err != nil {
				return err
			}
			ev, err := a.Parse(harness.PostTool, cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("signal hook: %w", err)
			}
			if ev.SessionID == "" {
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
			j, err := store.Load(ev.SessionID)
			if err != nil {
				return err
			}
			j.Turn++ // each PostToolUse advances the within-session clock

			switch {
			case ev.HasOutcome:
				j.RecordCommand(ev.Command, ev.Failed)
			case ev.FilePath != "":
				j.RecordEdit(relPath(e, ev.FilePath))
			}
			return store.Save(j)
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a PostToolUse event on stdin and record signals")
	cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot)")
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
