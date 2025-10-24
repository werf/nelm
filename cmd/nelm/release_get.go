package main

import (
	"cmp"
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type releaseGetConfig struct {
	action.ReleaseGetOptions

	LogColorMode     string
	LogLevel         string
	ReleaseName      string
	ReleaseNamespace string
}

func newReleaseGetCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseGetConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"get [options...] -n namespace -r release [revision]",
		"Get information about a deployed release.",
		"Get information about a deployed release.",
		20,
		releaseCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = log.SetupLogging(ctx, cmp.Or(log.Level(cfg.LogLevel), action.DefaultReleaseGetLogLevel), log.SetupLoggingOptions{
				ColorMode:      cfg.LogColorMode,
				LogIsParseable: true,
			})

			if len(args) > 0 {
				var err error

				cfg.Revision, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid revision: %s", args[0])
				}
			}

			if _, err := action.ReleaseGet(ctx, cfg.ReleaseName, cfg.ReleaseNamespace, cfg.ReleaseGetOptions); err != nil {
				return fmt.Errorf("release get: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddKubeConnectionFlags(cmd, &cfg.KubeConnectionOptions); err != nil {
			return fmt.Errorf("add kube connection flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NetworkParallelism, "network-parallelism", common.DefaultNetworkParallelism, "Limit of network-related tasks to run in parallel", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict values
		if err := cli.AddFlag(cmd, &cfg.OutputFormat, "output-format", action.DefaultReleaseGetOutputFormat, "Result output format", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.PrintValues, "print-values", false, "Print Values of the last Helm release", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict allowed values
		if err := cli.AddFlag(cmd, &cfg.ReleaseStorageDriver, "release-storage", "", "How releases should be stored", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseStorageSQLConnection, "release-storage-sql-connection", "", "SQL connection string for MySQL release storage driver", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
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

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", common.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", string(action.DefaultReleaseGetLogLevel), "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseName, "release", "", "The release name. Must be unique within the release namespace", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "r",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseNamespace, "namespace", "", "The release namespace. Resources with no namespace will be deployed here", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "n",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
