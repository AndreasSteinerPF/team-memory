package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/harness"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func newSignalCmd(g *globalOpts) *cobra.Command {
	var hook bool
	var prompt bool
	var harnessName string
	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Record nudge signals from a PostToolUse or UserPromptSubmit event (use --hook)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !hook {
				return fmt.Errorf("signal currently supports only --hook mode")
			}
			a, err := harness.Get(harnessName)
			if err != nil {
				return err
			}
			if prompt {
				return recordPromptSignal(cmd, g, a)
			}
			ev, err := a.Parse(harness.PostTool, cmd.InOrStdin())
			if err != nil {
				return fmt.Errorf("signal hook: %w", err)
			}
			if ev.SessionID == "" {
				return nil // cannot key a journal without a session
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
			j.Turn++ // each PostToolUse advances the within-session clock

			switch {
			case ev.HasOutcome:
				j.RecordCommand(ev.Command, ev.Failed)
			case ev.FilePath != "":
				j.RecordEdit(relPath(e, ev.FilePath))
			}

			var decision harness.Decision
			// Advisory injection runs only on non-Claude harnesses: Claude Code
			// already injects advisory memories PRE-edit via check-action, so
			// injecting again post-edit would double-surface. On claude this block
			// is skipped and the empty Decision renders nothing — signal recording
			// above still happens.
			if a.Name() != "claude" && ev.FilePath != "" {
				rel := relPath(e, ev.FilePath)
				if res, rerr := e.engine().Retrieve(retrieve.Query{Paths: []string{rel}}); rerr == nil {
					max := e.pol.Inject.AdvisoryMaxPerSession
					var fresh []retrieve.Result
					for _, r := range res {
						// Skip requirements (blocked pre-tool, not advised post-tool)
						// and anything already injected this session.
						if r.Memory.Enforcement == model.EnforcementRequirement {
							continue
						}
						if j.AlreadyInjected(r.Memory.ID) {
							continue
						}
						if len(j.Injected) >= max {
							break
						}
						fresh = append(fresh, r)
						j.MarkInjected(r.Memory.ID)
						j.RecordSurfaced(r.Memory.ID, rel, hasDrift(r))
					}
					if len(fresh) > 0 {
						decision.Context = buildContext(fresh)
					}
				}
			}
			if err := store.Save(j); err != nil {
				return err
			}
			return a.Render(harness.PostTool, decision, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&hook, "hook", false, "read a PostToolUse event on stdin and record signals")
	cmd.Flags().BoolVar(&prompt, "prompt", false, "record a UserPromptSubmit marker instead of a tool outcome (use with --hook)")
	cmd.Flags().StringVar(&harnessName, "harness", "claude", "harness adapter (claude, codex, copilot, cursor, gemini)")
	return cmd
}

// recordPromptSignal handles UserPromptSubmit: it records a prompt marker and
// advances the turn clock so the prompt sits between the surrounding edits,
// which is what the user-intervened signal keys on (prd.md §10.1, §10.6).
func recordPromptSignal(cmd *cobra.Command, g *globalOpts, a harness.Adapter) error {
	ev, err := a.Parse(harness.PromptSubmit, cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("signal hook: %w", err)
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
	j.Turn++ // the prompt occupies its own turn, between the edits around it
	j.RecordPrompt()

	// Drain nudges queued at Stop (Claude only — Stop-hook stdout doesn't
	// surface on Claude, so the nudge command parks the text in j.Pending and
	// this UserPromptSubmit injection re-delivers it via additionalContext,
	// the channel that does reach the agent. See ledger memory
	// 01KV84H0XQTPVWVNR65PG1TD2A.
	if err := store.Save(j); err != nil {
		return err
	}
	if len(j.Pending) == 0 {
		return nil
	}
	if err := a.Render(harness.PromptSubmit, harness.Decision{Context: strings.Join(j.Pending, "\n")}, cmd.OutOrStdout()); err != nil {
		return err
	}
	j.Pending = nil
	j.MarkQueuedDrained(j.Turn, time.Now().UTC())
	// Rendering happens before persistence, so a save failure may re-deliver
	// the context on the next prompt. This intentionally favors at-least-once
	// advisory delivery over silently losing a nudge.
	return store.Save(j)
}

// relPath converts an absolute or repo-relative path to a forward-slash repo path.
// When p is already repo-relative (not absolute), filepath.Abs resolves it against
// the process CWD which may differ from the repo dir; if the result escapes the
// repo root (starts with ".."), we fall back to using p as-is.
func relPath(e *env, p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		if r, err := filepath.Rel(e.repoDir, abs); err == nil {
			slash := filepath.ToSlash(r)
			if !strings.HasPrefix(slash, "../") && slash != ".." {
				return slash
			}
		}
	}
	return strings.TrimPrefix(filepath.ToSlash(p), "./")
}

// hasDrift reports whether a result carries any drift annotation.
func hasDrift(r retrieve.Result) bool {
	for _, d := range r.Drift {
		if d.Note != "" {
			return true
		}
	}
	return false
}
