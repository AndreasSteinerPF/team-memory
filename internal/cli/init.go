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

// printSetup prints integration next-steps. Installs the PreToolUse hook into
// .claude/settings.json when .claude/ is present.
func printSetup(w io.Writer, repoDir, remote string) {
	installed, err := installClaudeCodeHook(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hook: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed PreToolUse hook in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hook already present in .claude/settings.json.")
	}
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w, "  • MCP (Claude Code / Cursor / Codex): add to your .mcp.json —")
	fmt.Fprintln(w, `      { "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } } }`)
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote configured: %s (run `tm sync --remote %s`).\n", remote, remote)
	}
}
