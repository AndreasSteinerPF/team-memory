package cli

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newProposeCmd(g *globalOpts) *cobra.Command {
	var title, summary, guidance, origin, actor, session, ctxBranch string
	var scope, evidence, anchors, ctxPaths []string
	cmd := &cobra.Command{
		Use:   "propose <type>",
		Short: "Create a memory (type: failed_attempt|constraint|fragile_area|stale_doc|decision)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mt := model.MemoryType(args[0])
			if !validType(mt) {
				return fmt.Errorf("unknown type %q (want failed_attempt|constraint|fragile_area|stale_doc|decision)", args[0])
			}
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()

			m := model.Memory{
				Type:     mt,
				Origin:   model.ConstraintOrigin(origin),
				Title:    title,
				Summary:  summary,
				Guidance: guidance,
				Scope:    model.Scope{Paths: scope},
				Actor:    agentActor(actor, session),
			}
			for _, ev := range evidence {
				m.Evidence = append(m.Evidence, parseEvidence(ev))
			}
			for _, a := range anchors {
				an, err := parseAnchor(e.git, a)
				if err != nil {
					return err
				}
				m.Anchors = append(m.Anchors, an)
			}
			if ctxBranch != "" || len(ctxPaths) > 0 {
				m.CodeContext = &model.CodeContext{Branch: ctxBranch, Paths: ctxPaths}
			}

			warnSimilar(cmd, e, title)

			id, err := e.led.AppendMemory(m)
			if err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			triggerBackgroundPush(e)
			m.ID = id
			st := derive.Derive(m, nil, e.pol)
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, id)
			fmt.Fprintln(out, stateLine(st))
			fmt.Fprintf(out, "reason: %s\n", st.Reason)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "short title (required)")
	cmd.Flags().StringVar(&summary, "summary", "", "what happened")
	cmd.Flags().StringVar(&guidance, "guidance", "", "what a future agent should do")
	cmd.Flags().StringArrayVar(&scope, "scope", nil, "path glob this memory applies to (repeatable)")
	cmd.Flags().StringArrayVar(&evidence, "evidence", nil, "evidence as type:ref (repeatable)")
	cmd.Flags().StringArrayVar(&anchors, "anchor", nil, "anchor as path@ref, e.g. file@HEAD (repeatable)")
	cmd.Flags().StringVar(&origin, "origin", "", "constraint origin: team|external")
	cmd.Flags().StringVar(&actor, "actor", "cli", "actor name")
	cmd.Flags().StringVar(&session, "session", envSession(), "session id (defaults to $CLAUDE_SESSION_ID)")
	cmd.Flags().StringVar(&ctxBranch, "ctx-branch", "", "code-context branch")
	cmd.Flags().StringArrayVar(&ctxPaths, "ctx-path", nil, "code-context path (repeatable)")
	_ = cmd.MarkFlagRequired("title")
	return cmd
}

func validType(t model.MemoryType) bool {
	switch t {
	case model.TypeFailedAttempt, model.TypeConstraint, model.TypeFragileArea, model.TypeStaleDoc, model.TypeDecision:
		return true
	}
	return false
}

// warnSimilar searches the FTS index for memories similar to title and prints a
// warning to stderr when matches exist, prompting the user to confirm instead of
// creating a duplicate (prd.md §15 spam mitigation).
func warnSimilar(cmd *cobra.Command, e *env, title string) {
	q := ftsQuery(title)
	if q == "" {
		return
	}
	ids, err := e.idx.SearchIDs(q)
	if err != nil || len(ids) == 0 {
		return
	}
	const maxWarn = 3
	w := cmd.ErrOrStderr()
	fmt.Fprintln(w, "Note: similar memories already exist — consider confirming one instead of creating a duplicate:")
	shown := 0
	for _, id := range ids {
		if shown >= maxWarn {
			break
		}
		m, ok, err := e.led.Memory(id)
		if err != nil || !ok {
			continue
		}
		fmt.Fprintf(w, "  %s  %s  (`tm observe %s confirm`)\n", id, m.Title, id)
		shown++
	}
}

// ftsQuery builds a SQLite FTS5 query from s. It extracts alphanumeric tokens
// ≥5 chars, selects the 3 longest (most distinctive), and joins them with
// implicit AND so only memories sharing those key terms are flagged. Short or
// common words are excluded to avoid over-broad matches.
func ftsQuery(s string) string {
	var words []string
	for _, field := range strings.Fields(s) {
		var b strings.Builder
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		if b.Len() >= 5 {
			words = append(words, b.String())
		}
	}
	if len(words) == 0 {
		return ""
	}
	sort.Slice(words, func(i, j int) bool { return len(words[i]) > len(words[j]) })
	const maxTerms = 3
	if len(words) > maxTerms {
		words = words[:maxTerms]
	}
	return strings.Join(words, " ")
}
