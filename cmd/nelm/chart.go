package main

import (
	"context"

	"github.com/spf13/cobra"
)

func BuildChartCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chart",
		Short:   "Manage Helm Charts.",
		Long:    "Manage Helm Charts.",
		GroupID: ChartGroup.ID,
	}

	cmd.AddCommand(NewChartRenderCommand(ctx))

	return cmd
}
