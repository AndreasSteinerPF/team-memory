package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/derive"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
	"github.com/AndreasSteinerPF/team-memory/internal/retrieve"
)

func newListCmd(g *globalOpts) *cobra.Command {
	var stale, contested, staleCand, all, duplicateOnly, supersededOnly, pendingSupersedeOnly bool
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
			// Only build the pending-supersede set when the corresponding
			// filter is requested — keeps the default `tm list` cheap
			// (prd.md §10.5).
			var pendingSupersedeIDs map[string]bool
			if pendingSupersedeOnly {
				ms, mErr := e.led.Memories()
				if mErr != nil {
					return mErr
				}
				obs, oErr := e.led.Observations()
				if oErr != nil {
					return oErr
				}
				dctx := derive.BuildContext(ms, obs, e.pol)
				pendingSupersedeIDs = make(map[string]bool)
				for _, mm := range ms {
					if len(dctx.PendingSupersedeFor(mm.ID)) > 0 {
						pendingSupersedeIDs[mm.ID] = true
					}
				}
			}
			out := cmd.OutOrStdout()
			gd := retrieve.GitDrift{Git: e.git}
			n := 0
			for _, m := range rows {
				if !listMatch(m, stale, contested, staleCand, all, duplicateOnly, supersededOnly, pendingSupersedeOnly, gd, minDrift, pendingSupersedeIDs) {
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
	cmd.Flags().BoolVar(&duplicateOnly, "duplicate", false, "only memories marked as duplicate of another")
	cmd.Flags().BoolVar(&supersededOnly, "superseded", false, "only memories superseded by a newer canonical")
	cmd.Flags().BoolVar(&pendingSupersedeOnly, "pending-supersede", false, "only memories named as 'supersedes' in pending (unsubstantiated) supersede observations")
	cmd.Flags().IntVar(&minDrift, "min-drift", 10, "commits-changed threshold for --stale-candidates")
	return cmd
}

func listMatch(m index.IndexedMemory, stale, contested, staleCand, all, duplicateOnly, supersededOnly, pendingSupersedeOnly bool, gd retrieve.GitDrift, minDrift int, pendingSupersedeIDs map[string]bool) bool {
	switch {
	case stale:
		return m.Status == model.StatusStale
	case contested:
		return m.Status == model.StatusContested
	case duplicateOnly:
		return m.Status == model.StatusDuplicate
	case supersededOnly:
		return m.Status == model.StatusSuperseded
	case pendingSupersedeOnly:
		return pendingSupersedeIDs[m.ID]
	case staleCand:
		return heavilyDrifted(m, gd, minDrift)
	case all:
		return true
	default:
		// Default view excludes the four non-retrievable statuses, matching
		// retrieve's exclusion list (prd.md §10.5 / §8.2).
		return m.Status != model.StatusStale && m.Status != model.StatusRejected &&
			m.Status != model.StatusDuplicate && m.Status != model.StatusSuperseded
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
