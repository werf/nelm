package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/internal/tschart"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type chartInitConfig struct {
	TS bool
}

func newChartInitCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartInitConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"init [PATH]",
		"Initialize a new chart.",
		"Initialize a new chart in the specified directory. If PATH is not specified, uses the current directory.",
		10, // priority for ordering in help
		chartCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, log.InfoLevel, log.SetupLoggingOptions{})

			// Determine chart path
			chartPath := "."
			if len(args) > 0 {
				chartPath = args[0]
			}

			// Convert to absolute path for reliable name extraction
			absPath, err := filepath.Abs(chartPath)
			if err != nil {
				return fmt.Errorf("get absolute path: %w", err)
			}

			// Derive chart name from directory name
			chartName := filepath.Base(absPath)

			// Check --ts flag
			if !cfg.TS {
				return fmt.Errorf("non-TypeScript chart initialization not implemented yet, use --ts flag")
			}

			// Check feature gate
			if !featgate.FeatGateTypescript.Enabled() {
				log.Default.Warn(ctx, "TypeScript charts require NELM_FEAT_TYPESCRIPT=true environment variable")
				return fmt.Errorf("TypeScript charts feature is not enabled. Set NELM_FEAT_TYPESCRIPT=true to use this feature")
			}

			// Create directory if it doesn't exist
			if err := os.MkdirAll(absPath, 0755); err != nil {
				return fmt.Errorf("create directory %s: %w", absPath, err)
			}

			// Create chart structure (Chart.yaml, values.yaml, .helmignore)
			if err := tschart.InitChartStructure(ctx, absPath, chartName); err != nil {
				return fmt.Errorf("init chart structure: %w", err)
			}

			// Create TypeScript boilerplate (ts/ directory with all files)
			if err := tschart.InitTSBoilerplate(ctx, absPath, chartName); err != nil {
				return fmt.Errorf("init TypeScript boilerplate: %w", err)
			}

			// Ensure .gitignore has TypeScript entries
			if err := tschart.EnsureGitignore(absPath); err != nil {
				return fmt.Errorf("ensure .gitignore: %w", err)
			}

			log.Default.Info(ctx, "Initialized TypeScript chart in %s", absPath)
			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.TS, "ts", false, "Initialize TypeScript chart", cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}
		return nil
	}

	return cmd
}
