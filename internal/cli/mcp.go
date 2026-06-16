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
			// A harness can register `tm mcp` once and reuse it across every
			// repo (Codex stores MCP servers globally in ~/.codex/config.toml;
			// .mcp.json travels with the repo but `tm init` may simply never
			// have been run). Failing here would surface as a broken MCP
			// server in every unrelated session. Mirror brief's hook-safety
			// posture and serve a degraded server whose tools return an
			// IsError pointing the user at `tm init`.
			e, err := openEnv(g)
			if err != nil {
				srv := tmmcp.New(tmmcp.Deps{})
				return srv.Run(cmd.Context())
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
