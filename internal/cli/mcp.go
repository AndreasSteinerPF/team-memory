package cli

import (
	"github.com/spf13/cobra"

	tmmcp "github.com/AndreasSteinerPF/team-memory/internal/mcp"
)

func newMCPCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the TeamMemory MCP server (stdio)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			store, err := e.ackStore()
			if err != nil {
				return err
			}
			srv := tmmcp.New(tmmcp.Deps{
				Ledger:   e.led,
				Index:    e.idx,
				Policy:   e.pol,
				Engine:   e.engine(),
				AckStore: store,
			})
			return srv.Run(cmd.Context())
		},
	}
}
