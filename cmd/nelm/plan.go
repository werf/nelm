package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newPlanCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "plan",
		Short:   "Show planned changes.",
		Long:    "Show planned changes.",
		GroupID: planCmdGroup.ID,
	}

	cmd.AddCommand(newPlanDeployCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
