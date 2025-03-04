package main

import (
	"context"
	"fmt"

	cobra "github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
)

type versionConfig struct {
	outputFormat string
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
			}); err != nil {
				return fmt.Errorf("version: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
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
