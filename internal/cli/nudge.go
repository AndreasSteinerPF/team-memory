package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

type stopHookInput struct {
	SessionID string `json:"session_id"`
}

// stopHookOutput injects the nudge as additional context at turn end.
//
// VERIFY (spec §10): confirm the Stop-hook context-injection shape on the
// installed Claude Code version against a live payload. Some versions surface
// Stop stdout directly; others require {"decision":"block","reason":...} (which
// forces a turn — undesirable for a low-pressure nudge). This output struct
// isolates that decision to one place; adjust here if the live payload differs.
type stopHookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

func newNudgeCmd(g *globalOpts) *cobra.Command {
	var hook bool
	cmd := &cobra.Command{
		Use:   "nudge",
		Short: "Emit a proposing/observing nudge from a Stop event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("nudge currently supports only --hook mode")
			}
			var in stopHookInput
			if err := json.NewDecoder(cmd.InOrStdin()).Decode(&in); err != nil {
				return fmt.Errorf("nudge hook: decode stdin: %w", err)
			}
			if in.SessionID == "" {
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
			j, err := store.Load(in.SessionID)
			if err != nil {
				return err
			}

			acted := e.actedPredicate(in.SessionID)
			n, ok := nudge.Decide(j, e.nudgeConfig(), acted)
			if !ok {
				return nil // stay silent
			}

			// Record the fired nudge for dedup + budget, then persist.
			j.Fired = append(j.Fired, nudge.FiredNudge{Key: n.Key, Turn: j.Turn})
			if err := store.Save(j); err != nil {
				return err
			}

			var out stopHookOutput
			out.HookSpecificOutput.HookEventName = "Stop"
			out.HookSpecificOutput.AdditionalContext = n.Text
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a Stop event on stdin and emit at most one nudge")
	return cmd
}

// actedPredicate returns a function reporting whether this session has already
// proposed/observed for a signal — the suppress-if-acted rule (spec §4). It
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
