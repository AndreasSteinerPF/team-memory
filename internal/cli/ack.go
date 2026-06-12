package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newAckCmd(g *globalOpts) *cobra.Command {
	var note, session string
	var ttl time.Duration
	cmd := &cobra.Command{
		Use:   "ack <memory-id>",
		Short: "Acknowledge a requirement memory so matching edits proceed (local-only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			store, err := e.ackStore()
			if err != nil {
				return err
			}
			if err := store.Ack(args[0], session, note, ttl, time.Now().UTC()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "acknowledged %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "optional note recorded with the ack")
	cmd.Flags().StringVar(&session, "session", envSession(), "session id (defaults to $CLAUDE_SESSION_ID)")
	cmd.Flags().DurationVar(&ttl, "ttl", 8*time.Hour, "ack lifetime when no session id is available")
	return cmd
}
