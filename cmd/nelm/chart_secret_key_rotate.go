package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartSecretKeyRotateOptions struct {
	action.SecretKeyRotateOptions

	LogColorMode string
	LogLevel     string
}

func newChartSecretKeyRotateCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretKeyRotateOptions{}

	cmd := cli.NewSubCommand(
		ctx,
		"rotate [options...] --old-secret-key secret-key --new-secret-key secret-key [chart-dir]",
		"Reencrypt secret files with a new secret key.",
		"Decrypt with an old secret key, then encrypt with a new secret key chart files secret-values.yaml and secret/*.",
		70,
		secretCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultSecretKeyRotateLogLevel), log.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
			})

			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.SecretKeyRotate(ctx, cfg.SecretKeyRotateOptions); err != nil {
				return fmt.Errorf("secret key rotate: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", action.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultSecretKeyRotateLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NewSecretKey, "new-secret-key", "", "New secret key", cli.AddFlagOptions{
			Group:    mainFlagGroup,
			Required: true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.OldSecretKey, "old-secret-key", "", "Old secret key", cli.AddFlagOptions{
			Group:    mainFlagGroup,
			Required: true,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretValuesFiles, "secret-values", []string{}, "Secret values files paths", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
