package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartSecretKeyCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Manage chart secret keys.",
		Long:  "Manage chart secret keys.",
	}

	cmd.AddCommand(newChartSecretKeyCreateCommand(ctx, afterAllCommandsBuiltFuncs))
	cmd.AddCommand(newChartSecretKeyRotateCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
