package cli

import (
	"fmt"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
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
			if remote == "" {
				remote = e.ledgerRemote()
			}
			res, syncErr := e.led.Sync(remote)
			if syncErr != nil {
				// The push-failure store is already populated by openEnv's
				// callback. Read it back and print the same diagnosis the
				// status/doctor surfaces use.
				if store, oerr := git.OpenPushFailureStore(e.gitDir); oerr == nil {
					if rec, rerr := store.Read(); rerr == nil && rec != nil {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"sync: push to %q failed (%s).\n  Fix: %s\n",
							rec.Remote, pushFailureHumanKind(rec.Kind), pushFailureFixHint(rec))
					}
				}
				return syncErr
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sync: %s\n", res.Action)
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "", "git remote (or URL/path) to sync with (default: git config tm.remote, else origin)")
	return cmd
}
