package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "chart",
		Short:   "Manage Helm Charts.",
		Long:    "Manage Helm Charts.",
		GroupID: chartCmdGroup.ID,
	}

	cmd.AddCommand(newChartRenderCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDependencyCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDownloadCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartUploadCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartArchiveCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
