package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartSecretFileCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Manage chart secret files.",
		Long:  "Manage chart secret files.",
	}

	cmd.AddCommand(newChartSecretFileEncryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretFileDecryptCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretFileEditCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
