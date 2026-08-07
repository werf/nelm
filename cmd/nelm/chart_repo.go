package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartRepoCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"repo",
		"Manage chart repositories.",
		"Manage chart repositories.",
		repoCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartRepoAddCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartRepoRemoveCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartRepoUpdateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartRepoLoginCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartRepoLogoutCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
