package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newChartSecretCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Manage chart secrets.",
		Long:  "Manage chart secrets.",
	}

	cmd.AddCommand(newChartSecretKeyCommand(ctx, afterAllCommandsBuiltFuncs))

	return cmd
}
