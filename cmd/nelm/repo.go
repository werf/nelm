package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newRepoCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "repo",
		Short:   "Manage chart repositories.",
		Long:    "Manage chart repositories.",
		GroupID: repoCmdGroup.ID,
	}

	cmd.AddCommand(newRepoAddCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoRemoveCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoUpdateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoLoginCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoLogoutCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
