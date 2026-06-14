package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func newInitCmd(g *globalOpts) *cobra.Command {
	var remote string
	var harnessName string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the ledger branch, default policy, and local index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Validate harness flag before any I/O so unknown values always error.
			switch harnessName {
			case "", "claude", "codex", "copilot", "cursor", "gemini":
				// valid
			default:
				return fmt.Errorf("unknown harness %q", harnessName)
			}

			repoDir, err := filepath.Abs(g.repo)
			if err != nil {
				return err
			}
			led, err := ledger.Open(repoDir, g.branch)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !led.Exists() {
				py, err := policy.DefaultYAML()
				if err != nil {
					return err
				}
				if err := led.Init(py); err != nil {
					return err
				}
				if remote != "" {
					// env isn't open yet (the ledger was just created), so run git
					// directly rather than through e.git.
					if _, err := (git.Runner{Dir: repoDir}).Run("config", "tm.remote", remote); err != nil {
						return err
					}
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
			} else {
				fmt.Fprintf(out, "ledger already initialized on branch %q\n", g.branch)
			}
			switch harnessName {
			case "", "claude":
				printSetup(out, repoDir, remote)
			case "codex":
				if err := installCodex(repoDir); err != nil {
					return err
				}
				fmt.Fprintln(out, "Installed Codex plugin in .codex-plugin/ (hooks + MCP server).")
			case "copilot":
				if err := installCopilot(repoDir, out); err != nil {
					return err
				}
				fmt.Fprintln(out, "Installed Copilot hooks in .github/hooks/teammemory.json.")
			case "cursor":
				if err := installCursor(repoDir); err != nil {
					return err
				}
				fmt.Fprintln(out, "Installed Cursor hooks in .cursor/ (hooks + rule + MCP).")
			case "gemini":
				if err := installGemini(repoDir); err != nil {
					return err
				}
				fmt.Fprintln(out, "Installed Gemini CLI settings in .gemini/settings.json (hooks + MCP) and ensured GEMINI.md.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "", "optional separate remote for the ledger branch")
	cmd.Flags().StringVar(&harnessName, "harness", "", "install hooks for this harness (claude, codex, copilot, cursor, gemini)")
	return cmd
}

// printSetup prints integration next-steps. Installs Claude Code hooks into
// .claude/settings.json when .claude/ is present.
func printSetup(w io.Writer, repoDir, remote string) {
	installed, err := installClaudeCodeHooks(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hooks: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed Claude Code hooks (PreToolUse check + SessionStart brief) in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hooks already present in .claude/settings.json.")
	}
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w, "  • MCP (Claude Code / Cursor / Codex): add to your .mcp.json —")
	fmt.Fprintln(w, `      { "mcpServers": { "teammemory": { "command": "tm", "args": ["mcp"] } } }`)
	if remote != "" {
		fmt.Fprintf(w, "  • Ledger remote stored as git config tm.remote=%s; sync and background fetch/push use it.\n", remote)
	}
}
