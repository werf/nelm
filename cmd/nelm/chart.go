package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chart",
		Short:   "Manage charts.",
		Long:    "Manage charts.",
		GroupID: chartCmdGroup.ID,
	}

	cmd.AddCommand(newChartRenderCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDependencyCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDownloadCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartUploadCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartPackCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartLintCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
