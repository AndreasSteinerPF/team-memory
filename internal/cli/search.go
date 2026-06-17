package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func newSearchCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Lexical search over the ledger (title/summary/guidance)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			out := cmd.OutOrStdout()
			raw := strings.Join(args, " ")
			q := retrieve.FTSQuery(raw)
			if q == "" {
				if strings.TrimSpace(raw) != "" {
					fmt.Fprintln(out, "Query has no searchable tokens — search uses lexical keywords, not glob patterns. Try a word from a memory title, or run `tm status` to see what's in the ledger.")
					return nil
				}
				fmt.Fprintln(out, "No results.")
				return nil
			}
			ids, err := e.idx.SearchIDs(q)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				fmt.Fprintln(out, "No results.")
				return nil
			}
			rows, err := e.idx.All()
			if err != nil {
				return err
			}
			byID := make(map[string]index.IndexedMemory, len(rows))
			for _, m := range rows {
				byID[m.ID] = m
			}
			for _, id := range ids {
				if m, ok := byID[id]; ok {
					fmt.Fprintf(out, "%s  [%s]  %s\n", m.ID, m.Status, m.Title)
				}
			}
			return nil
		},
	}
}
