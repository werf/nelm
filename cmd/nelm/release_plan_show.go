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

type releasePlanShowConfig struct {
	action.ReleasePlanShowOptions

	LogColorMode string
	LogLevel     string
}

func newReleasePlanShowCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releasePlanShowConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"show [options...] plan.json",
		"Show plan artifact planned changes.",
		"Show plan artifact planned changes.",
		20,
		releaseCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.ExactArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveDefault
			},
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultReleasePlanExecuteLogLevel), log.SetupLoggingOptions{
				ColorMode: cfg.LogColorMode,
			})

			cfg.PlanArtifactPath = args[0]

			if err := action.ReleasePlanShow(ctx, cfg.ReleasePlanShowOptions); err != nil {
				return fmt.Errorf("plan show: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.ShowInsignificantDiffs, "show-insignificant-diffs", false, "Show insignificant diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowSensitiveDiffs, "show-sensitive-diffs", false, "Show sensitive diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowVerboseCRDDiffs, "show-verbose-crd-diffs", false, "Show verbose CRD diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(v2): get rid?
		if err := cli.AddFlag(cmd, &cfg.ShowVerboseDiffs, "show-verbose-diffs", true, "Show verbose diff lines", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}
		
		if err := cli.AddFlag(cmd, &cfg.SecretKey, "secret-key", "", "Secret key for decrypting the plan artifact", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretWorkDir, "secret-work-dir", "", "Working directory for secret operations", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "Temporary directory for operation", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "log-color-mode", "auto", "Set log color mode. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultReleasePlanExecuteLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
