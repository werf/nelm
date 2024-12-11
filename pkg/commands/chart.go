package commands

import (
	"github.com/spf13/cobra"
)

func NewChartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chart",
		Short:   "Manage Helm charts",
		Long:    "Manage Helm charts",
	}

	cmd.AddCommand(NewChartRenderCommand())

	return cmd
}
