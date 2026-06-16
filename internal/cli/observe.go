package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newObserveCmd(g *globalOpts) *cobra.Command {
	var summary, actor, session, ctxBranch string
	var evidence, scope, scopeCommand, ctxPaths, ctxCommands []string
	cmd := &cobra.Command{
		Use:   "observe <memory-id> <kind>",
		Short: "Add an observation (kind: confirm|contradict|adjust_scope|mark_stale)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			kind := model.ObservationKind(args[1])
			if !validAgentKind(kind) {
				return fmt.Errorf("unknown or human-only kind %q (use confirm|contradict|adjust_scope|mark_stale)", args[1])
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			if _, ok, err := e.led.Memory(target); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("no memory %s", target)
			}

			o := model.Observation{
				Target:  target,
				Kind:    kind,
				Summary: summary,
				Actor:   agentActor(actor, session),
			}
			for _, ev := range evidence {
				o.Evidence = append(o.Evidence, parseEvidence(ev))
			}
			if kind == model.KindAdjustScope {
				if len(scope) == 0 && len(scopeCommand) == 0 {
					return fmt.Errorf("adjust_scope requires --scope or --scope-command")
				}
				o.SuggestedScope = &model.Scope{Paths: scope, Commands: scopeCommand}
			}
			if ctxBranch != "" || len(ctxPaths) > 0 || len(ctxCommands) > 0 {
				o.CodeContext = &model.CodeContext{Branch: ctxBranch, Paths: ctxPaths, Commands: ctxCommands}
			}

			if _, err := e.led.AppendObservation(o); err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			triggerBackgroundPush(e)
			return printTargetState(cmd.OutOrStdout(), e, target)
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "what you observed")
	cmd.Flags().StringArrayVar(&evidence, "evidence", nil, "evidence as type:ref (repeatable)")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "suggested scope glob for adjust_scope (repeatable)")
	cmd.Flags().StringVar(&actor, "actor", "cli", "actor name")
	cmd.Flags().StringVar(&session, "session", envSession(), "session id (defaults to $CLAUDE_SESSION_ID)")
	cmd.Flags().StringVar(&ctxBranch, "ctx-branch", "", "code-context branch")
	cmd.Flags().StringArrayVar(&ctxPaths, "ctx-path", nil, "code-context path (repeatable)")
	cmd.Flags().StringArrayVar(&scopeCommand, "scope-command", nil, "suggested command pattern for adjust_scope (repeatable)")
	cmd.Flags().StringArrayVar(&ctxCommands, "ctx-command", nil, "code-context command you ran (repeatable; substantiates command broadening)")
	return cmd
}

func validAgentKind(k model.ObservationKind) bool {
	switch k {
	case model.KindConfirm, model.KindContradict, model.KindAdjustScope, model.KindMarkStale:
		return true
	}
	return false
}

// printTargetState re-derives a memory from its full observation set and prints
// its current state. Shared by observe, approve, and reject.
func printTargetState(w io.Writer, e *env, target string) error {
	m, ok, err := e.led.Memory(target)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no memory %s", target)
	}
	obs, err := e.led.Observations()
	if err != nil {
		return err
	}
	ms, err := e.led.Memories()
	if err != nil {
		return err
	}
	ctx := derive.BuildContext(ms, obs, e.pol)
	st := derive.DeriveWithContext(m, observationsFor(obs, target), e.pol, ctx)
	fmt.Fprintln(w, target)
	fmt.Fprintln(w, stateLine(st))
	fmt.Fprintf(w, "reason: %s\n", st.Reason)
	return nil
}
