package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/git"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/ledger"
	"github.com/AndreasSteinerPF/team-memory/internal/policy"
)

func newInitCmd(g *globalOpts) *cobra.Command {
	var remote string
	var harnessName string
	var noPush bool
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
				adopted, err := adoptFetchedLedgerBranch(repoDir, g.branch)
				if err != nil {
					return err
				}
				if adopted {
					led, err = ledger.Open(repoDir, g.branch)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "Adopted fetched TeamMemory ledger on branch %q.\n", g.branch)
				} else {
					py, err := policy.DefaultYAML()
					if err != nil {
						return err
					}
					if err := led.Init(py); err != nil {
						return err
					}
					// Resolve the candidate remote: explicit --remote wins; otherwise
					// default to "origin" if the repo has one configured.
					candidate := remote
					if candidate == "" {
						if _, err := (git.Runner{Dir: repoDir}).Run("remote", "get-url", "origin"); err == nil {
							candidate = "origin"
						}
					}

					if candidate != "" && !noPush {
						if vErr := git.ValidateRemote(repoDir, candidate, 5*time.Second); vErr != nil {
							if remote != "" {
								fmt.Fprintf(out, "Remote %q not reachable (%v); did not store tm.remote.\n", remote, vErr)
								fmt.Fprintln(out, "Fix the URL, then `tm remote set <value>`.")
							} else {
								fmt.Fprintf(out, "origin not reachable (%v); ledger created locally.\n", vErr)
							}
							candidate = "" // skip the push below
						} else if remote != "" {
							// env isn't open yet (the ledger was just created), so run git
							// directly rather than through e.git.
							if _, err := (git.Runner{Dir: repoDir}).Run("config", "tm.remote", remote); err != nil {
								return err
							}
						}
					}

					if candidate != "" && !noPush {
						// Best-effort push to seed the remote ref so teammates can fetch
						// it. Use a raw `git push` (not led.Sync) so init does NOT pull
						// remote state into the freshly-created orphan ledger — init's
						// job is to seed, not to reconcile (prd.md §7.4).
						ref := "refs/heads/" + g.branch
						_, perr := (git.Runner{Dir: repoDir}).Run("push", "--quiet", candidate, ref+":"+ref)
						if perr == nil {
							fmt.Fprintf(out, "Pushed ledger branch to %s. Teammates can fetch it now.\n", candidate)
						} else {
							// openEnv's callback isn't installed yet (we did not call
							// openEnv during init). Record directly so tm status/doctor
							// see the failure on the next invocation.
							gitDir, _ := led.GitDir()
							if store, oerr := git.OpenPushFailureStore(gitDir); oerr == nil {
								kind := git.ClassifyPushStderr(perr.Error())
								_ = store.Record(candidate, kind, perr.Error(), time.Now().UTC())
								if kind == git.KindProtectedBranch {
									fmt.Fprintf(out, "%s rejects the teammemory branch (branch protection).\n", candidate)
									fmt.Fprintln(out, "Fix: exempt 'teammemory' from protection rules,")
									fmt.Fprintln(out, "     or run: tm remote set git@host:org/repo-memory.git")
								} else {
									fmt.Fprintf(out, "Push deferred: %v. Will retry on next propose/observe/sync.\n", perr)
								}
							}
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
				}
			} else {
				fmt.Fprintf(out, "ledger already initialized on branch %q\n", g.branch)
			}
			switch harnessName {
			case "", "claude":
				printSetup(out, repoDir, remote)
			case "codex":
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				if err := installCodex(repoDir, home, out); err != nil {
					return err
				}
				fmt.Fprintln(out, "Installed Codex hooks in .codex/hooks.json.")
			case "copilot":
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				if err := installCopilot(repoDir, home, out); err != nil {
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
	cmd.Flags().BoolVar(&noPush, "no-push", false, "do not validate or push to the ledger remote (offline/bootstrap)")
	return cmd
}

// printSetup prints integration next-steps. Installs Claude Code hooks into
// .claude/settings.json when .claude/ is present, and registers the teammemory
// MCP server in the repo-root .mcp.json (merge-safe).
func printSetup(w io.Writer, repoDir, remote string) {
	installed, err := installClaudeCodeHooks(repoDir)
	if err != nil {
		fmt.Fprintf(w, "Warning: could not install Claude Code hooks: %v\n", err)
	} else if installed {
		fmt.Fprintln(w, "Installed Claude Code hooks (PreToolUse check + SessionStart brief) in .claude/settings.json.")
	} else if _, serr := os.Stat(filepath.Join(repoDir, ".claude")); serr == nil {
		fmt.Fprintln(w, "Claude Code hooks already present in .claude/settings.json.")
	}
	// printSetup's contract is to print next-steps and never fail init, so an
	// MCP-registration error here is a warning, not a hard error (unlike the
	// --harness paths, which abort).
	mcpPath := filepath.Join(repoDir, ".mcp.json")
	if added, err := ensureMCPServerJSON(mcpPath, map[string]any{"command": "tm", "args": []string{"mcp"}}); err != nil {
		fmt.Fprintf(w, "Warning: could not register MCP server in .mcp.json: %v\n", err)
	} else if added {
		fmt.Fprintln(w, "Registered teammemory MCP server in .mcp.json.")
	} else {
		fmt.Fprintln(w, "teammemory MCP server already registered in .mcp.json.")
	}
	_ = remote
}
