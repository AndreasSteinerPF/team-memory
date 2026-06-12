package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newShowCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "show <memory-id>",
		Short: "Show a memory's envelope, observations, and derived state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
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
			rel := observationsFor(obs, target)
			st := derive.Derive(m, rel, e.pol)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s  %s\n", m.ID, m.Title)
			fmt.Fprintf(out, "type: %s", m.Type)
			if m.Origin != "" {
				fmt.Fprintf(out, " (origin: %s)", m.Origin)
			}
			fmt.Fprintln(out)
			if m.Summary != "" {
				fmt.Fprintf(out, "summary: %s\n", m.Summary)
			}
			if m.Guidance != "" {
				fmt.Fprintf(out, "guidance: %s\n", m.Guidance)
			}
			fmt.Fprintf(out, "scope: %s\n", strings.Join(m.Scope.Paths, ", "))
			if !sameScope(m.Scope.Paths, st.EffectiveScope.Paths) {
				fmt.Fprintf(out, "effective scope: %s\n", strings.Join(st.EffectiveScope.Paths, ", "))
			}
			fmt.Fprintln(out, stateLine(st))
			fmt.Fprintf(out, "reason: %s\n", st.Reason)
			fmt.Fprintf(out, "independent confirms: %d   contradictions: %d\n", st.IndependentConfirms, st.Contradictions)
			for _, a := range m.Anchors {
				fmt.Fprintf(out, "anchor: %s @ %s\n", a.Path, shortSHA(a.Commit))
			}
			if len(rel) > 0 {
				fmt.Fprintln(out, "observations:")
				for _, o := range rel {
					fmt.Fprintf(out, "  %s  %s  (%s)\n", o.CreatedAt.UTC().Format(time.RFC3339), o.Kind, o.Actor.Kind)
					if o.Kind == model.KindAdjustScope && o.SuggestedScope != nil {
						fmt.Fprintf(out, "    suggested scope: %s\n", strings.Join(o.SuggestedScope.Paths, ", "))
					}
				}
			}
			return nil
		},
	}
}

func sameScope(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
