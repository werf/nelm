package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartDependencyCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"dependency",
		"Manage chart dependencies.",
		"Manage chart dependencies.",
		dependencyCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartDependencyUpdateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartDependencyDownloadCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
