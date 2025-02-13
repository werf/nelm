package main

import (
	"context"

	"github.com/spf13/cobra"
)

func BuildRootCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nelm",
		Long:          "Nelm is a Helm 3 replacement. Nelm manages and deploys Helm Charts to Kubernetes just like Helm, but provides a lot of features, improvements and bug fixes on top of what Helm 3 offers.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddGroup(
		ReleaseGroup,
		PlanGroup,
		ChartGroup,
	)

	cmd.AddCommand(BuildReleaseCommand(ctx))
	cmd.AddCommand(BuildPlanCommand(ctx))
	cmd.AddCommand(BuildChartCommand(ctx))

	return cmd
}
