package main

import (
	"context"

	"github.com/spf13/cobra"
)

func BuildPlanCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plan",
		Short:   "Show planned changes.",
		Long:    "Show planned changes.",
		GroupID: PlanGroup.ID,
	}

	cmd.AddCommand(NewPlanDeployCommand(ctx))

	return cmd
}
