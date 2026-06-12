package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/export"
	"github.com/AndreasSteinerPF/team-memory/internal/index"
	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newExportCmd(g *globalOpts) *cobra.Command {
	var format, outPath string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Generate AGENTS.md / CLAUDE.md / .cursor/rules blocks or JSON from active memories",
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
			var active []index.IndexedMemory
			for _, m := range rows {
				if m.Status == model.StatusActive {
					active = append(active, m)
				}
			}

			var data []byte
			switch format {
			case "json":
				data, err = export.JSON(active)
				if err != nil {
					return err
				}
				data = append(data, '\n')
			case "agents", "claude", "cursor":
				data = []byte(export.Markdown(active, "Project memory (TeamMemory)", export.Instructions(format)))
			default:
				return fmt.Errorf("unknown --format %q (want agents|claude|cursor|json)", format)
			}

			if outPath != "" {
				return os.WriteFile(outPath, data, 0o644)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	cmd.Flags().StringVar(&format, "format", "agents", "agents | claude | cursor | json")
	cmd.Flags().StringVar(&outPath, "out", "", "write to this file instead of stdout")
	return cmd
}
