package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSyncCmd(g *globalOpts) *cobra.Command {
	var remote string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch, union-merge, and push the ledger branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			res, err := e.led.Sync(remote)
			if err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sync: %s\n", res.Action)
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote (or path) to sync with")
	return cmd
}
