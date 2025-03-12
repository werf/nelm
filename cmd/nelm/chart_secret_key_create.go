package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretKeyCreateOptions struct {
	logLevel string
}

func (c *chartSecretKeyCreateOptions) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartSecretKeyCreateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretKeyCreateOptions{}

	cmd := &cobra.Command{
		Use:                   "create [options...]",
		Short:                 "Create a new chart secret key.",
		Long:                  "Create a new chart secret key.",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := action.SecretKeyCreate(ctx, action.SecretKeyCreateOptions{
				LogLevel: cfg.LogLevel(),
			}); err != nil {
				return fmt.Errorf("secret key create: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultSecretKeyCreateLogLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
