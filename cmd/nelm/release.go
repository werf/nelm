package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newReleaseCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "release",
		Short:   "Manage Helm releases.",
		Long:    "Manage Helm releases.",
		GroupID: releaseGroup.ID,
	}

	cmd.AddCommand(newReleaseDeployCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newReleaseUninstallCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
