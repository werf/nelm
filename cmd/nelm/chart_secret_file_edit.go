package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
)

type chartSecretFileEditOptions struct {
	action.SecretFileEditOptions

	File     string
	LogLevel string
}

func newChartSecretFileEditCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartSecretFileEditOptions{}

	cmd := cli.NewSubCommand(
		ctx,
		"edit [options...] --secret-key secret-key file",
		"Interactively edit encrypted file.",
		"Interactively edit encrypted file.",
		30,
		secretCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.ExactArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveDefault
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = action.SetupLogging(ctx, cfg.LogLevel, action.DefaultSecretFileEditLogLevel)

			cfg.File = args[0]

			if err := action.SecretFileEdit(ctx, cfg.File, cfg.SecretFileEditOptions); err != nil {
				return fmt.Errorf("secret file edit: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", action.DefaultLogColorMode, "Color mode for logs", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", action.DefaultSecretFileEditLogLevel, "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretKey, "secret-key", "", "Secret key", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
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
