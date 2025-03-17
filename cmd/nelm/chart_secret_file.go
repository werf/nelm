package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartSecretFileCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"file",
		"Manage chart secret files.",
		"Manage chart secret files.",
		secretCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartSecretFileEncryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretFileDecryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretFileEditCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
