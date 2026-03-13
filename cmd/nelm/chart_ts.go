package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartTSCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"ts",
		"Manage TypeScript charts.",
		"Manage TypeScript charts.",
		tsCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartTSInitCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartTSBuildCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
