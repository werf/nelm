package main

import (
	"context"

	"github.com/spf13/cobra"
)

func BuildReleaseCommand(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "release",
		Short:   "Manage Helm releases.",
		Long:    "Manage Helm releases.",
		GroupID: ReleaseGroup.ID,
	}

	cmd.AddCommand(BuildReleaseDeployCommand(ctx))
	cmd.AddCommand(NewReleaseUninstallCommand(ctx))

	return cmd
}
