package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartDependencyCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dependency",
		Short: "Manage chart dependencies.",
		Long:  "Manage chart dependencies.",
	}

	cmd.AddCommand(newChartDependencyUpdateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDependencyBuildCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
