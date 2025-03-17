package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartSecretCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"secret",
		"Manage chart secrets.",
		"Manage chart secrets.",
		secretCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartSecretKeyCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretFileCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretValuesFileCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
