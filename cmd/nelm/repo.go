package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newRepoCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"repo",
		"Manage chart repositories.",
		"Manage chart repositories.",
		repoCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newRepoAddCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoRemoveCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoUpdateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoLoginCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newRepoLogoutCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
