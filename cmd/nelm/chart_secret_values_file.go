package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartSecretValuesFileCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values-file",
		Short: "Manage chart secret values files.",
		Long:  "Manage chart secret values files.",
	}

	cmd.AddCommand(newChartSecretValuesFileEncryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretValuesFileDecryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretValuesFileEditCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
