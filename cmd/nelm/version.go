package main

import (
	"context"
	"fmt"

	cobra "github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type versionConfig struct {
	TempDirPath string

	logColorMode string
	logLevel     string
	outputFormat string
}

func (c *versionConfig) LogColorMode() action.LogColorMode {
	return action.LogColorMode(c.logColorMode)
}

func (c *versionConfig) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func (c *versionConfig) OutputFormat() common.OutputFormat {
	return common.OutputFormat(c.outputFormat)
}

func newVersionCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &versionConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"version [options...]",
		"Show version.",
		"Show version.",
		50,
		miscCmdGroup,
		cli.SubCommandOptions{},
		func(cmd *cobra.Command, args []string) error {
			if _, err := action.Version(ctx, action.VersionOptions{
				LogColorMode: cfg.LogColorMode(),
				LogLevel:     cfg.LogLevel(),
				OutputFormat: cfg.OutputFormat(),
				TempDirPath:  cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("version: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.logColorMode, "color-mode", string(action.DefaultLogColorMode), "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.logLevel, "log-level", string(action.DefaultVersionLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.outputFormat, "output-format", string(action.DefaultVersionOutputFormat), "Result output format", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
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
