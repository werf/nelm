package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
)

func newChartSecretValuesFileCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := cli.NewGroupCommand(
		ctx,
		"values-file",
		"Manage chart secret values files.",
		"Manage chart secret values files.",
		secretCmdGroup,
		cli.GroupCommandOptions{},
	)

	cmd.AddCommand(newChartSecretValuesFileEncryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretValuesFileDecryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretValuesFileEditCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
