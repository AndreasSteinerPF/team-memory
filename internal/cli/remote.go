package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	tmgit "github.com/AndreasSteinerPF/team-memory/internal/git"
)

// newRemoteCmd implements `tm remote {show|set|unset}` — a first-class surface
// for managing git config tm.remote (the separate-remote mode, prd.md §7.1).
func newRemoteCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Show, set, or unset the ledger remote (prd.md §7.1 separate-remote mode)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newRemoteShowCmd(g),
		newRemoteSetCmd(g),
		newRemoteUnsetCmd(g),
	)
	// Bare `tm remote` aliases to `tm remote show`.
	cmd.RunE = newRemoteShowCmd(g).RunE
	return cmd
}

func newRemoteShowCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the current ledger remote and its source",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			gr := tmgit.Runner{Dir: repoDir}
			out := cmd.OutOrStdout()
			configured := ""
			if v, err := gr.Run("config", "--get", "tm.remote"); err == nil {
				configured = strings.TrimSpace(v)
			}
			name := configured
			if name == "" {
				name = "origin"
			}
			resolvedURL := ""
			if !strings.ContainsAny(name, "/:\\") {
				if v, err := gr.Run("remote", "get-url", name); err == nil {
					resolvedURL = strings.TrimSpace(v)
				}
			}
			switch {
			case configured == "" && resolvedURL != "":
				fmt.Fprintf(out, "Ledger remote: %s (default)  →  %s\n", name, resolvedURL)
				fmt.Fprintln(out, "Source:        none configured")
			case configured == "" && resolvedURL == "":
				fmt.Fprintf(out, "Ledger remote: %s (default)  (no such git remote)\n", name)
				fmt.Fprintln(out, "Source:        none configured")
				fmt.Fprintln(out, "Hint:          run `git remote add origin <url>` or `tm remote set <url>`.")
			case strings.ContainsAny(configured, "/:\\"):
				fmt.Fprintf(out, "Ledger remote: %s\n", configured)
				fmt.Fprintln(out, "Source:        git config tm.remote")
			case resolvedURL != "":
				fmt.Fprintf(out, "Ledger remote: %s  →  %s\n", configured, resolvedURL)
				fmt.Fprintln(out, "Source:        git config tm.remote")
			default:
				fmt.Fprintf(out, "Ledger remote: %s  (unknown — not a registered git remote)\n", configured)
				fmt.Fprintln(out, "Source:        git config tm.remote")
				fmt.Fprintf(out, "Hint:          run `git remote add %s <url>` or `tm remote set <url>`.\n", configured)
			}
			return nil
		},
	}
}

func newRemoteSetCmd(g *globalOpts) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "set <name-or-url>",
		Short: "Set git config tm.remote to <name-or-url> after validating it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			value := args[0]
			if !force {
				if err := tmgit.ValidateRemote(repoDir, value, 5*time.Second); err != nil {
					return fmt.Errorf("validation failed (run with --force to skip): %w", err)
				}
			}
			gr := tmgit.Runner{Dir: repoDir}
			if _, err := gr.Run("config", "tm.remote", value); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Set git config tm.remote = %s\n", value)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip ls-remote validation")
	return cmd
}

func newRemoteUnsetCmd(g *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "unset",
		Short: "Remove git config tm.remote (reverts to origin)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			gr := tmgit.Runner{Dir: repoDir}
			// `git config --unset` returns exit 5 if the key is absent — make
			// this idempotent: succeed either way.
			_, _ = gr.Run("config", "--unset", "tm.remote")
			fmt.Fprintln(cmd.OutOrStdout(), "Ledger remote reverted to origin (default).")
			return nil
		},
	}
}
