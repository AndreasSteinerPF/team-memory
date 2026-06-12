package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newProposeCmd(g *globalOpts) *cobra.Command {
	var title, summary, guidance, origin, actor, session, ctxBranch string
	var scope, evidence, anchors, ctxPaths []string
	cmd := &cobra.Command{
		Use:   "propose <type>",
		Short: "Create a memory (type: failed_attempt|constraint|fragile_area|stale_doc|decision)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mt := model.MemoryType(args[0])
			if !validType(mt) {
				return fmt.Errorf("unknown type %q (want failed_attempt|constraint|fragile_area|stale_doc|decision)", args[0])
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()

			m := model.Memory{
				Type:     mt,
				Origin:   model.ConstraintOrigin(origin),
				Title:    title,
				Summary:  summary,
				Guidance: guidance,
				Scope:    model.Scope{Paths: scope},
				Actor:    agentActor(actor, session),
			}
			for _, ev := range evidence {
				m.Evidence = append(m.Evidence, parseEvidence(ev))
			}
			for _, a := range anchors {
				an, err := parseAnchor(e.git, a)
				if err != nil {
					return err
				}
				m.Anchors = append(m.Anchors, an)
			}
			if ctxBranch != "" || len(ctxPaths) > 0 {
				m.CodeContext = &model.CodeContext{Branch: ctxBranch, Paths: ctxPaths}
			}

			id, err := e.led.AppendMemory(m)
			if err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			m.ID = id
			st := derive.Derive(m, nil, e.pol)
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, id)
			fmt.Fprintln(out, stateLine(st))
			fmt.Fprintf(out, "reason: %s\n", st.Reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "short title (required)")
	cmd.Flags().StringVar(&summary, "summary", "", "what happened")
	cmd.Flags().StringVar(&guidance, "guidance", "", "what a future agent should do")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "path glob this memory applies to (repeatable)")
	cmd.Flags().StringArrayVar(&evidence, "evidence", nil, "evidence as type:ref (repeatable)")
	cmd.Flags().StringArrayVar(&anchors, "anchor", nil, "anchor as path@ref, e.g. file@HEAD (repeatable)")
	cmd.Flags().StringVar(&origin, "origin", "", "constraint origin: team|external")
	cmd.Flags().StringVar(&actor, "actor", "cli", "actor name")
	cmd.Flags().StringVar(&session, "session", envSession(), "session id (defaults to $CLAUDE_SESSION_ID)")
	cmd.Flags().StringVar(&ctxBranch, "ctx-branch", "", "code-context branch")
	cmd.Flags().StringArrayVar(&ctxPaths, "ctx-path", nil, "code-context path (repeatable)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func validType(t model.MemoryType) bool {
	switch t {
	case model.TypeFailedAttempt, model.TypeConstraint, model.TypeFragileArea, model.TypeStaleDoc, model.TypeDecision:
		return true
	}
	return false
}
