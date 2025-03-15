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
	TempDirPath string

	logColorMode string
	logLevel     string
}

func (c *chartSecretKeyCreateOptions) LogColorMode() action.LogColorMode {
	return action.LogColorMode(c.logColorMode)
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
		ValidArgsFunction:     cobra.NoFileCompletions,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := action.SecretKeyCreate(ctx, action.SecretKeyCreateOptions{
				LogColorMode: cfg.LogColorMode(),
				LogLevel:     cfg.LogLevel(),
				TempDirPath:  cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("secret key create: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logColorMode, "color-mode", string(action.DefaultLogColorMode), "Color mode for logs", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultSecretKeyCreateLogLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			Group: miscFlagGroup,
			Type:  flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
