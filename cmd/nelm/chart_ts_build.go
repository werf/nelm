package main

import (
	"cmp"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type chartTSBuildConfig struct {
	action.ChartTSBuildOptions

	LogColorMode string
	LogLevel     string
}

func newChartTSBuildCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartTSBuildConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"build [PATH]",
		"Build vendor for typescript chart.",
		"Build vendor for typescript chart in the specified directory. If PATH is not specified, uses the current directory.",
		10, // priority for ordering in help
		tsCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), log.InfoLevel), log.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
			})

			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.ChartTSBuild(ctx, cfg.ChartTSBuildOptions); err != nil {
				return fmt.Errorf("chart build: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(log.InfoLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
