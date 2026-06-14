package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func newCheckActionCmd(g *globalOpts) *cobra.Command {
	var paths []string
	var command, desc, provMode string
	var hook bool
	var harnessName string
	cmd := &cobra.Command{
		Use:   "check-action",
		Short: "Surface memories relevant to an action (use --hook for the PreToolUse hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			// Trigger a non-blocking background fetch when the last fetch is stale
			// (prd.md §7.4). Never waits on the network — hook latency unaffected.
			maybeTriggerFetch(e)
			if hook {
				a, err := harness.Get(harnessName)
				if err != nil {
					return err
				}
				return runHook(cmd, e, a)
			}
			res, err := e.engine().Retrieve(retrieve.Query{
				Paths: paths, Command: command, Description: desc, ProvisionalMode: provMode,
			})
			if err != nil {
				return err
			}
			printResults(cmd.OutOrStdout(), res)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&paths, "path", nil, "action target path (repeatable)")
	cmd.Flags().StringVar(&command, "command", "", "the command being run (matched against scope.commands)")
	cmd.Flags().StringVar(&desc, "description", "", "free-text action description (FTS)")
	cmd.Flags().StringVar(&provMode, "provisional-mode", "", "never | related | always (default: policy)")
	cmd.Flags().BoolVar(&hook, "hook", false, "read a PreToolUse event on stdin and emit a hook decision")
	cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot)")
	return cmd
}

// printResults renders the human-readable check-action output.
func printResults(w io.Writer, res []retrieve.Result) {
	if len(res) == 0 {
		fmt.Fprintln(w, "No relevant memories.")
		return
	}
	for _, r := range res {
		m := r.Memory
		tag := string(m.Enforcement)
		if r.Provisional {
			tag = "provisional/" + tag
		}
		fmt.Fprintf(w, "• [%s] %s (%s)\n", tag, m.Title, m.ID)
		if g := firstNonEmpty(m.Guidance, m.Summary); g != "" {
			fmt.Fprintf(w, "    %s\n", g)
		}
		if r.Caution != "" {
			fmt.Fprintf(w, "    %s\n", r.Caution)
		}
		if r.Request != "" {
			fmt.Fprintf(w, "    %s\n", r.Request)
		}
		for _, d := range r.Drift {
			if d.Note != "" {
				fmt.Fprintf(w, "    drift: %s\n", d.Note)
			}
		}
		if m.Enforcement == model.EnforcementRequirement {
			fmt.Fprintf(w, "    requirement — run the checks, then `tm ack %s` and retry.\n", m.ID)
		}
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// maybeTriggerFetch fires a detached background `git fetch` of the ledger
// branch when the last fetch is older than policy.Sync.AutoFetchAfter (prd.md
// §7.4). The hook never waits on the network — this function returns immediately
// after starting the subprocess.
func maybeTriggerFetch(e *env) {
	interval := 5 * time.Minute
	if d, err := time.ParseDuration(e.pol.Sync.AutoFetchAfter); err == nil && d > 0 {
		interval = d
	}

	stampFile := filepath.Join(e.gitDir, "tm", "last_fetch")
	if data, err := os.ReadFile(stampFile); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			if time.Since(t) < interval {
				return // still fresh
			}
		}
	}

	// Write the stamp before checking the remote so concurrent invocations
	// don't pile up processes, and so the interval is respected even when
	// no remote is configured.
	_ = os.WriteFile(stampFile, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644)

	// Only start the subprocess when a remote is configured. This avoids
	// creating git lock files in repos without a remote (e.g. tests), which
	// would race with temporary-directory cleanup.
	remote := e.ledgerRemote()
	if !e.remoteAvailable(remote) {
		return
	}

	ref := "refs/heads/" + e.branch
	cmd := exec.Command("git", "-C", e.repoDir, "fetch", "--quiet", "--no-tags",
		remote, ref+":"+ref)
	// Start detached — intentionally not calling Wait; parent may exit first.
	_ = cmd.Start()
}

// --- hook mode ---

func runHook(cmd *cobra.Command, e *env, a harness.Adapter) error {
	ev, err := a.Parse(harness.PreTool, cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("check-action hook: %w", err)
	}
	var q retrieve.Query
	switch {
	case ev.FilePath != "":
		rel := ev.FilePath
		if abs, err := filepath.Abs(rel); err == nil {
			if r, err := filepath.Rel(e.repoDir, abs); err == nil {
				rel = filepath.ToSlash(r)
			}
		}
		q.Paths = []string{rel}
	case ev.Command != "":
		q.Command = ev.Command
	default:
		return nil // nothing to check
	}

	res, err := e.engine().Retrieve(q)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		return nil // emit nothing; the edit proceeds
	}

	// Record each surfaced memory into the nudge journal so observe signals
	// have a source (spec §6.1). Pure side-effect: must not alter hook output.
	if nstore, nerr := e.nudgeStore(); nerr == nil && ev.SessionID != "" {
		if j, lerr := nstore.Load(ev.SessionID); lerr == nil {
			for _, r := range res {
				drift := false
				for _, d := range r.Drift {
					if d.Note != "" {
						drift = true
					}
				}
				path := ""
				if len(r.Memory.EffectiveScope) > 0 {
					path = r.Memory.EffectiveScope[0]
				}
				j.RecordSurfaced(r.Memory.ID, path, drift)
			}
			_ = nstore.Save(j)
		}
	}

	store, err := e.ackStore()
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	var blockers, context []retrieve.Result
	for _, r := range res {
		if r.Memory.Enforcement == model.EnforcementRequirement && r.Memory.Status == model.StatusActive {
			acked, err := store.IsAcked(r.Memory.ID, ev.SessionID, now)
			if err != nil {
				return err
			}
			if !acked {
				blockers = append(blockers, r)
				continue
			}
		}
		context = append(context, r)
	}

	if len(blockers) > 0 {
		return a.Render(harness.PreTool, harness.Decision{Block: true, Reason: buildBlockReason(blockers)}, cmd.OutOrStdout())
	}
	if len(context) > 0 {
		return a.Render(harness.PreTool, harness.Decision{Context: buildContext(context)}, cmd.OutOrStdout())
	}
	return nil
}

func buildBlockReason(rs []retrieve.Result) string {
	var b strings.Builder
	b.WriteString("TeamMemory: blocked by unacknowledged requirement(s).\n")
	for _, r := range rs {
		fmt.Fprintf(&b, "Requirement (mem %s): %s\n", r.Memory.ID, r.Memory.Title)
		if r.Memory.Guidance != "" {
			fmt.Fprintf(&b, "  %s\n", r.Memory.Guidance)
		}
		fmt.Fprintf(&b, "  Run the required checks, then `tm ack %s` and retry.\n", r.Memory.ID)
	}
	return b.String()
}

func buildContext(rs []retrieve.Result) string {
	var b strings.Builder
	b.WriteString("TeamMemory — relevant memories for this action:\n")
	for _, r := range rs {
		fmt.Fprintf(&b, "- [%s] %s\n", r.Memory.Enforcement, r.Memory.Title)
		if r.Memory.Guidance != "" {
			fmt.Fprintf(&b, "  %s\n", r.Memory.Guidance)
		}
		if r.Caution != "" {
			fmt.Fprintf(&b, "  %s\n", r.Caution)
		}
		for _, d := range r.Drift {
			if d.Note != "" {
				fmt.Fprintf(&b, "  drift: %s\n", d.Note)
			}
		}
	}
	return b.String()
}
