package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newStatusCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Ledger overview and items needing human attention",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			rows, err := e.idx.All()
			if err != nil {
				return err
			}
			counts := map[model.Status]int{}
			var contested, criticalProv []index.IndexedMemory
			for _, m := range rows {
				counts[m.Status]++
				if m.Status == model.StatusContested {
					contested = append(contested, m)
				}
				if m.Status == model.StatusProvisional && m.Risk == model.RiskCritical {
					criticalProv = append(criticalProv, m)
				}
			}

			// Pending supersede claims are cross-memory state, not stored on
			// the index row. Compute once via derive.BuildContext (prd.md §8.5).
			var pendingSupersede int
			ms, mErr := e.led.Memories()
			obs, oErr := e.led.Observations()
			if mErr == nil && oErr == nil {
				dctx := derive.BuildContext(ms, obs, e.pol)
				for _, m := range ms {
					if len(dctx.PendingSupersedeFor(m.ID)) > 0 {
						pendingSupersede++
					}
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Memories: %d active, %d provisional, %d contested, %d stale, %d duplicate, %d superseded, %d rejected\n",
				counts[model.StatusActive], counts[model.StatusProvisional],
				counts[model.StatusContested], counts[model.StatusStale],
				counts[model.StatusDuplicate], counts[model.StatusSuperseded],
				counts[model.StatusRejected])
			if pendingSupersede > 0 {
				fmt.Fprintf(out, "Pending supersede claims: %d\n", pendingSupersede)
			}
			if len(contested) > 0 {
				fmt.Fprintln(out, "\nContested (needs human attention):")
				for _, m := range contested {
					fmt.Fprintf(out, "  %s  %s\n", m.ID, m.Title)
				}
			}
			if len(criticalProv) > 0 {
				fmt.Fprintln(out, "\nCritical, awaiting human approval:")
				for _, m := range criticalProv {
					fmt.Fprintf(out, "  %s  %s\n", m.ID, m.Title)
				}
			}
			tip, _ := e.led.Tip()
			fmt.Fprintf(out, "\nLedger branch %q at %s\n", e.branch, shortSHA(tip))

			// Push-failure surface (spec §3.3): surface only consecutive >= 2 and recent.
			if store, perr := git.OpenPushFailureStore(e.gitDir); perr == nil {
				if rec, rerr := store.ReadFresh(time.Now().UTC(), 7*24*time.Hour); rerr == nil && rec != nil && rec.Consecutive >= 2 {
					fmt.Fprintln(out)
					fmt.Fprintf(out, "⚠ Last %d background pushes to %q rejected (%s).\n",
						rec.Consecutive, rec.Remote, pushFailureHumanKind(rec.Kind))
					fmt.Fprintf(out, "  Fix: %s\n", pushFailureFixHint(rec))
				}
			}
			return nil
		},
	}
}
