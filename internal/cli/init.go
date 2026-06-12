package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func newInitCmd(g *globalOpts) *cobra.Command {
	var remote string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the ledger branch, default policy, and local index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			led, err := ledger.Open(repoDir, g.branch)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if led.Exists() {
				fmt.Fprintf(out, "ledger already initialized on branch %q\n", g.branch)
				return nil
			}
			py, err := policy.DefaultYAML()
			if err != nil {
				return err
			}
			if err := led.Init(py); err != nil {
				return err
			}
			gitDir, err := led.GitDir()
			if err != nil {
				return err
			}
			idx, err := index.Open(index.PathFor(gitDir), led)
			if err != nil {
				return err
			}
			defer idx.Close()

			fmt.Fprintf(out, "Initialized TeamMemory ledger on branch %q.\n", g.branch)
			printSetup(out, repoDir, remote)
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "", "optional separate remote for the ledger branch")
	return cmd
}

// printSetup prints integration next-steps. The MCP server (Slice 6) and the
// Claude Code plugin/hook (Slice 7) are not wired here; this prints the config
// snippet and detection note only.
func printSetup(w io.Writer, repoDir, remote string) {
	if _, err := os.Stat(filepath.Join(repoDir, ".claude")); err == nil {
		fmt.Fprintln(w, "Detected a Claude Code project (.claude/).")
	}
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w, "  • MCP (Claude Code / Cursor / Codex): add to your .mcp.json —")
	fmt.Fprintln(w, `      { "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } } }`)
	fmt.Fprintln(w, "  • The Claude Code hook + plugin install ships with the plugin (later release).")
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote configured: %s (run `tm sync --remote %s`).\n", remote, remote)
	}
}
