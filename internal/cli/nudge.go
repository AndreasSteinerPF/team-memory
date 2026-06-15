package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func newNudgeCmd(g *globalOpts) *cobra.Command {
	var hook bool
	var harnessName string
	cmd := &cobra.Command{
		Use:   "nudge",
		Short: "Emit a proposing/observing nudge from a Stop event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("nudge currently supports only --hook mode")
			}
			a, err := harness.Get(harnessName)
			if err != nil {
				return err
			}
			ev, err := a.Parse(harness.Stop, cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("nudge hook: %w", err)
			}
			if ev.SessionID == "" {
				return nil
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

			acted := e.actedPredicate(ev.SessionID)
			n, ok := nudge.Decide(j, e.nudgeConfig(), acted)
			if !ok {
				return nil // stay silent
			}

			// Record the fired nudge for dedup + budget, then persist.
			j.Fired = append(j.Fired, nudge.FiredNudge{Key: n.Key, Turn: j.Turn})
			if err := store.Save(j); err != nil {
				return err
			}

			return a.Render(harness.Stop, harness.Decision{Context: n.Text}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a Stop event on stdin and emit at most one nudge")
	cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot, cursor, gemini)")
	return cmd
}

// actedPredicate returns a function reporting whether this session has already
// proposed/observed for a signal — the suppress-if-acted rule (prd.md §10.1). It
// checks the ledger for records authored by sessionID touching the signal's
// path (propose) or targeting the signal's memory (observe).
func (e *env) actedPredicate(sessionID string) func(nudge.Signal) bool {
	mems, err := e.led.Memories() // full model.Memory records (Actor + Scope); the
	if err != nil {               // index projection (IndexedMemory) lacks Actor.
		return func(nudge.Signal) bool { return false }
	}
	obs, _ := e.led.Observations()
	return func(s nudge.Signal) bool {
		if s.Verb == "observe" {
			for _, o := range obs {
				if o.Target == s.Memory && o.Actor.SessionID == sessionID {
					return true
				}
			}
			return false
		}
		for _, m := range mems {
			if m.Actor.SessionID != sessionID {
				continue
			}
			for _, p := range m.Scope.Paths {
				if p == s.Path {
					return true
				}
			}
		}
		return false
	}
}
