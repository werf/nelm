package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartSecretKeyCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"key",
		"Manage chart secret keys.",
		"Manage chart secret keys.",
		secretCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartSecretKeyCreateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretKeyRotateCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
