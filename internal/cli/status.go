package cli

import (
	"fmt"

	"github.com/spf13/cobra"

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
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Memories: %d active, %d provisional, %d contested, %d stale, %d rejected\n",
				counts[model.StatusActive], counts[model.StatusProvisional],
				counts[model.StatusContested], counts[model.StatusStale], counts[model.StatusRejected])
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
			return nil
		},
	}
}
