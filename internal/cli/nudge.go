package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/nudge"
)

func newNudgeCmd(g *globalOpts) *cobra.Command {
	var hook bool
	var harnessName string
	cmd := &cobra.Command{
		Use:   "nudge",
		Short: "Emit a proposing/observing nudge from a Stop event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("nudge currently supports only --hook mode")
			}
			a, err := harness.Get(harnessName)
			if err != nil {
				return err
			}
			ev, err := a.Parse(harness.Stop, cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("nudge hook: %w", err)
			}
			if ev.SessionID == "" {
				return nil
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			writeCurrentSession(e.gitDir, ev.SessionID)
			store, err := e.nudgeStore()
			if err != nil {
				return err
			}
			j, err := store.Load(ev.SessionID)
			if err != nil {
				return err
			}

			acted := e.actedPredicate(ev.SessionID)
			dec := nudge.Decide(j, e.nudgeConfig(), acted)
			if len(dec.Suppressions) > 0 {
				j.RecordSuppressions(dec.Suppressions)
			}
			if !dec.Fired {
				if len(dec.Suppressions) > 0 {
					return store.Save(j)
				}
				return nil // stay silent
			}

			n := dec.Nudge
			delivery := nudge.DeliveryRendered
			if a.Name() == "claude" || a.Name() == "codex" {
				delivery = nudge.DeliveryQueued
			}
			// Record the fired nudge for dedup, budget, and reporting.
			j.Fired = append(j.Fired, nudge.FiredFromNudge(n, j.Turn, delivery, time.Now().UTC()))
			// On Claude Code, Stop-hook stdout does not actually surface to the
			// agent's next turn (live-verified 2026-06-17). Codex Stop hooks are
			// also unsuitable for advisory context: plain text is rejected, and
			// additionalContext belongs on UserPromptSubmit. Queue the text here
			// so the next prompt hook re-injects it through the surfaced channel.
			if delivery == nudge.DeliveryQueued {
				j.Pending = append(j.Pending, n.Text)
			}
			if err := store.Save(j); err != nil {
				return err
			}

			return a.Render(harness.Stop, harness.Decision{Context: n.Text}, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a Stop event on stdin and emit at most one nudge")
	cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot, cursor, gemini)")
	cmd.AddCommand(newNudgeReportCmd(g))
	return cmd
}

func newNudgeReportCmd(g *globalOpts) *cobra.Command {
	var session string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Report local nudge outcome counts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			journals, warnings, err := loadNudgeJournals(e.gitDir, session)
			if err != nil {
				return err
			}
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), w)
			}
			mems, obs, ledgerOK := loadReportLedger(e)
			report := nudge.BuildReport(journals, mems, obs, ledgerOK)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			printNudgeReport(cmd.OutOrStdout(), report)
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "report one session id")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

func loadNudgeJournals(gitDir, session string) ([]nudge.Journal, []string, error) {
	dir := filepath.Join(gitDir, "tm", "nudge")
	if session != "" {
		j, err := loadNudgeJournalPath(filepath.Join(dir, session+".json"))
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, []string{fmt.Sprintf("Warning: skipped corrupt nudge journal %s: %v", session, err)}, nil
		}
		return []nudge.Journal{j}, nil, nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var journals []nudge.Journal
	var warnings []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		j, err := loadNudgeJournalPath(filepath.Join(dir, ent.Name()))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Warning: skipped corrupt nudge journal %s: %v", ent.Name(), err))
			continue
		}
		journals = append(journals, j)
	}
	return journals, warnings, nil
}

func loadNudgeJournalPath(path string) (nudge.Journal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nudge.Journal{}, err
	}
	var j nudge.Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nudge.Journal{}, err
	}
	return j, nil
}

func loadReportLedger(e *env) ([]model.Memory, []model.Observation, bool) {
	mems, merr := e.led.Memories()
	obs, oerr := e.led.Observations()
	if merr != nil || oerr != nil {
		return nil, nil, false
	}
	return mems, obs, true
}

func printNudgeReport(w io.Writer, r nudge.Report) {
	fmt.Fprintln(w, "Nudge report (.git/tm/nudge)")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Sessions: %d\n", r.Sessions)
	fmt.Fprintf(w, "Turns: %d\n", r.Turns)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Candidates:")
	fmt.Fprintf(w, "  detected: %d\n", r.Detected)
	fmt.Fprintf(w, "  fired: %d\n", r.Fired)
	fmt.Fprintf(w, "  suppressed: %d\n", r.Suppressed)
	for _, reason := range []string{"disabled", "max_per_session", "cooldown", "dedup", "already_acted"} {
		if n := r.SuppressedByReason[reason]; n > 0 {
			fmt.Fprintf(w, "    %s: %d\n", reason, n)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Delivery:")
	fmt.Fprintf(w, "  rendered: %d\n", r.Rendered)
	fmt.Fprintf(w, "  queued: %d\n", r.Queued)
	fmt.Fprintf(w, "  drained: %d\n", r.Drained)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Follow-through:")
	fmt.Fprintf(w, "  target-matched: %d\n", r.FollowThrough.TargetMatched)
	fmt.Fprintf(w, "  session-level: %d\n", r.FollowThrough.SessionLevel)
	fmt.Fprintf(w, "  none: %d\n", r.FollowThrough.None)
	fmt.Fprintf(w, "  unavailable: %d\n", r.FollowThrough.Unavailable)
}

// actedPredicate returns a function reporting whether this session has already
// proposed/observed for a signal — the suppress-if-acted rule (prd.md §10.1). It
// checks the ledger for records authored by sessionID touching the signal's
// path (propose) or targeting the signal's memory (observe).
func (e *env) actedPredicate(sessionID string) func(nudge.Signal) bool {
	mems, err := e.led.Memories() // full model.Memory records (Actor + Scope); the
	if err != nil {               // index projection (IndexedMemory) lacks Actor.
		return func(nudge.Signal) bool { return false }
	}
	obs, _ := e.led.Observations()
	return func(s nudge.Signal) bool {
		if s.Verb == "observe" {
			for _, o := range obs {
				if o.Target == s.Memory && o.Actor.SessionID == sessionID {
					return true
				}
			}
			return false
		}
		for _, m := range mems {
			if m.Actor.SessionID != sessionID {
				continue
			}
			for _, p := range m.Scope.Paths {
				if p == s.Path {
					return true
				}
			}
		}
		return false
	}
}
