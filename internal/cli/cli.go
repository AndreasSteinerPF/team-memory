// Package cli is the entry point for the tm command: a cobra application that
// exposes TeamMemory's 13 commands (prd.md §10.5) over the internal packages.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Version is the build version, overridable at link time via -ldflags.
var Version = "dev"

// globalOpts holds the persistent flags shared by every command.
type globalOpts struct {
	repo   string // path to the code repository (default ".")
	branch string // ledger branch name (default "teammemory")
}

// newRootCmd builds the full command tree. A fresh tree is built per invocation
// so flag state never leaks between Run calls (important for in-process tests).
func newRootCmd() *cobra.Command {
	g := &globalOpts{}
	root := &cobra.Command{
		Use:           "tm",
		Short:         "TeamMemory — a git-backed collaborative memory ledger for coding agents",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&g.repo, "repo", ".", "path to the code repository")
	root.PersistentFlags().StringVar(&g.branch, "branch", "teammemory", "ledger branch name")
	root.AddCommand(
		newVersionCmd(),
		newInitCmd(g),
		newProposeCmd(g),
		newObserveCmd(g),
		newApproveCmd(g),
		newRejectCmd(g),
		newListCmd(g),
		newShowCmd(g),
		newSearchCmd(g),
		// Subsequent tasks register their commands here.
	)
	return root
}

// Run executes the CLI against the given args and streams, returning a process
// exit code. cmd.OutOrStdout()/InOrStdin() throughout the tree resolve to these.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, "tm:", err)
		return 1
	}
	return 0
}

// Main wires Run to the real OS streams. cmd/tm and the e2e harness call it.
func Main() int {
	return Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the tm version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "team-memory %s\n", Version)
			return nil
		},
	}
}
