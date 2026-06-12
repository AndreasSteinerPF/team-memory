package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AndreasSteinerPF/team-memory/internal/model"
)

func newApproveCmd(g *globalOpts) *cobra.Command {
	var enforcement, confidence, summary, actor string
	cmd := &cobra.Command{
		Use:   "approve <memory-id>",
		Short: "Human: activate a memory and optionally raise enforcement/confidence",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			if _, ok, err := e.led.Memory(target); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("no memory %s", target)
			}
			o := model.Observation{
				Target:  target,
				Kind:    model.KindApprove,
				Summary: summary,
				Actor:   humanActor(actor),
			}
			if enforcement != "" {
				o.SetEnforcement = model.Enforcement(enforcement)
			}
			if confidence != "" {
				o.SetConfidence = model.Confidence(confidence)
			}
			if _, err := e.led.AppendObservation(o); err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			return printTargetState(cmd.OutOrStdout(), e, target)
		},
	}
	cmd.Flags().StringVar(&enforcement, "enforcement", "", "set enforcement: hint|recommendation|warning|requirement")
	cmd.Flags().StringVar(&confidence, "confidence", "", "set confidence: low|medium|high")
	cmd.Flags().StringVar(&summary, "summary", "", "approval note")
	cmd.Flags().StringVar(&actor, "actor", "human", "approver name")
	return cmd
}

func newRejectCmd(g *globalOpts) *cobra.Command {
	var summary, actor string
	cmd := &cobra.Command{
		Use:   "reject <memory-id>",
		Short: "Human: kill a memory (terminal)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			e, err := openEnv(g)
			if err != nil {
				return err
			}
			defer e.close()
			if _, ok, err := e.led.Memory(target); err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("no memory %s", target)
			}
			o := model.Observation{
				Target:  target,
				Kind:    model.KindReject,
				Summary: summary,
				Actor:   humanActor(actor),
			}
			if _, err := e.led.AppendObservation(o); err != nil {
				return err
			}
			if err := e.idx.Update(); err != nil {
				return err
			}
			return printTargetState(cmd.OutOrStdout(), e, target)
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "rejection note")
	cmd.Flags().StringVar(&actor, "actor", "human", "rejector name")
	return cmd
}
