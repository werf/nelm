package main

import (
	"context"
	"fmt"

	cobra "github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type versionConfig struct {
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

	cmd := &cobra.Command{
		Use:                   "version [options...]",
		Short:                 "Show version.",
		Long:                  "Show version.",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := action.Version(ctx, action.VersionOptions{
				OutputFormat: cfg.OutputFormat(),
				LogColorMode: cfg.LogColorMode(),
				LogLevel:     cfg.LogLevel(),
			}); err != nil {
				return fmt.Errorf("version: %w", err)
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
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultVersionLogLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.outputFormat, "output-format", string(action.DefaultVersionOutputFormat), "Result output format", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
