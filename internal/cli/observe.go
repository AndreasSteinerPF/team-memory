package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newObserveCmd(g *globalOpts) *cobra.Command {
	var summary, actor, session, ctxBranch, canonicalID, supersedes string
	var evidence, scope, scopeCommand, ctxPaths, ctxCommands []string
	cmd := &cobra.Command{
		Use:   "observe <memory-id> <kind>",
		Short: "Add an observation (kind: confirm|contradict|adjust_scope|mark_stale|mark_duplicate|supersede)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			kind := model.ObservationKind(args[1])
			if !validAgentKind(kind) {
				return fmt.Errorf("unknown or human-only kind %q (use confirm|contradict|adjust_scope|mark_stale|mark_duplicate|supersede)", args[1])
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
			if kind == model.KindMarkDuplicate {
				if canonicalID == "" {
					return fmt.Errorf("mark_duplicate requires --canonical-id")
				}
				if canonicalID == target {
					return fmt.Errorf("mark_duplicate canonical-id cannot equal target (file the observation on the duplicate, naming the kept memory in --canonical-id)")
				}
				if _, ok, err := e.led.Memory(canonicalID); err != nil {
					return err
				} else if !ok {
					return fmt.Errorf("canonical-id memory %s not found", canonicalID)
				}
				if cycle, err := detectCycle(e, target, canonicalID, model.KindMarkDuplicate); err != nil {
					return err
				} else if cycle {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"Note: a duplicate chain from %s already reaches %s — your observation would close a duplicate cycle. Every memory in the cycle will be hidden from default retrieval.\n",
						canonicalID, target)
				}
				warnIfNonActive(cmd, e, canonicalID)
				warnIfNonActive(cmd, e, target)
				o.CanonicalID = canonicalID
			}
			if kind == model.KindSupersede {
				if supersedes == "" {
					return fmt.Errorf("supersede requires --supersedes")
				}
				if supersedes == target {
					return fmt.Errorf("supersedes cannot equal target (file the observation on the new canonical, naming the obsolete one in --supersedes)")
				}
				if _, ok, err := e.led.Memory(supersedes); err != nil {
					return err
				} else if !ok {
					return fmt.Errorf("supersedes memory %s not found", supersedes)
				}
				if cycle, err := detectCycle(e, target, supersedes, model.KindSupersede); err != nil {
					return err
				} else if cycle {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"Note: a supersede chain from %s already reaches %s — your observation would close a supersede cycle. Every memory in the cycle is at risk of being hidden from default retrieval if claims substantiate.\n",
						supersedes, target)
				}
				warnIfNonActive(cmd, e, supersedes)
				warnIfNonActive(cmd, e, target)
				o.Supersedes = supersedes
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
	cmd.Flags().StringVar(&canonicalID, "canonical-id", "", "canonical memory ID for kind=mark_duplicate (required when kind is mark_duplicate)")
	cmd.Flags().StringVar(&supersedes, "supersedes", "", "obsolete memory ID for kind=supersede (required when kind is supersede; file the observation on the new canonical and name the obsolete one here)")
	return cmd
}

func validAgentKind(k model.ObservationKind) bool {
	switch k {
	case model.KindConfirm, model.KindContradict, model.KindAdjustScope,
		model.KindMarkStale, model.KindMarkDuplicate, model.KindSupersede:
		return true
	}
	return false
}

// warnIfNonActive prints a stderr warning if id refers to a memory that is
// currently rejected/stale/duplicate/superseded — the cross-memory reference
// may still be intentional (e.g. consolidating duplicates against a
// to-be-staled canonical) so we warn instead of blocking.
func warnIfNonActive(cmd *cobra.Command, e *env, id string) {
	st, ok, err := e.idx.Status(id)
	if err != nil || !ok {
		return
	}
	if st.IsNonActive() {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Note: referenced memory %s is currently %s — proceeding, but verify this is intentional.\n",
			id, st)
	}
}

// detectCycle reports whether b has an observation of `kind` pointing back at
// a. Used for one-hop cycle detection on mark_duplicate / supersede: filing
// M1->M2 when M2->M1 already exists would close a two-step cycle (prd.md §8.2,
// §8.5). Warn-not-block — the operator may be deliberately consolidating, but
// they should see the loop before committing it. Thin wrapper around
// derive.HasCycleBackTo that handles ledger access.
func detectCycle(e *env, a, b string, kind model.ObservationKind) (bool, error) {
	obs, err := e.led.Observations()
	if err != nil {
		return false, err
	}
	return derive.HasCycleBackTo(obs, a, b, kind), nil
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
