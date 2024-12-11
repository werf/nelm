package commands

import (
	"github.com/spf13/cobra"
)

func NewPlanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plan",
		Short:   "Plan a Helm chart",
		Long:    "Plan a Helm chart with the specified release name.",
	}

	cmd.AddCommand(NewPlanDeployCommand())

	return cmd
}
