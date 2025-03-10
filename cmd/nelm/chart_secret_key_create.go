package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/nelm/pkg/action"
)

type chartSecretKeyCreateOptions struct{}

func newChartSecretKeyCreateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	_ = &chartSecretKeyCreateOptions{}

	cmd := &cobra.Command{
		Use:                   "create [options...]",
		Short:                 "Create a new chart secret key.",
		Long:                  "Create a new chart secret key.",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := action.SecretKeyCreate(ctx, action.SecretKeyCreateOptions{}); err != nil {
				return fmt.Errorf("secret key create: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		return nil
	}

	return cmd
}
