package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func newListCmd(g *globalOpts) *cobra.Command {
	var stale, contested, staleCand, all bool
	var minDrift int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memories (default: active + provisional + contested)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			rows, err := e.idx.All()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			gd := retrieve.GitDrift{Git: e.git}
			n := 0
			for _, m := range rows {
				if !listMatch(m, stale, contested, staleCand, all, gd, minDrift) {
					continue
				}
				fmt.Fprintf(out, "%s  [%s/%s]  %s\n", m.ID, m.Status, m.Enforcement, m.Title)
				n++
			}
			if n == 0 {
				fmt.Fprintln(out, "No matching memories.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&stale, "stale", false, "only stale memories")
	cmd.Flags().BoolVar(&contested, "contested", false, "only contested memories")
	cmd.Flags().BoolVar(&staleCand, "stale-candidates", false, "only memories whose anchored files drifted heavily")
	cmd.Flags().BoolVar(&all, "all", false, "include stale and rejected memories")
	cmd.Flags().IntVar(&minDrift, "min-drift", 10, "commits-changed threshold for --stale-candidates")
	return cmd
}

func listMatch(m index.IndexedMemory, stale, contested, staleCand, all bool, gd retrieve.GitDrift, minDrift int) bool {
	switch {
	case stale:
		return m.Status == model.StatusStale
	case contested:
		return m.Status == model.StatusContested
	case staleCand:
		return heavilyDrifted(m, gd, minDrift)
	case all:
		return true
	default:
		return m.Status != model.StatusStale && m.Status != model.StatusRejected
	}
}

// heavilyDrifted reports whether any anchored file is missing or has changed at
// least minDrift commits since the anchor (prd.md §8.6 cleanup surface).
func heavilyDrifted(m index.IndexedMemory, gd retrieve.GitDrift, minDrift int) bool {
	for _, a := range m.Anchors {
		exists, changed, err := gd.Drift(a.Path, a.Commit)
		if err != nil {
			continue
		}
		if !exists || (changed >= 0 && changed >= minDrift) {
			return true
		}
	}
	return false
}
