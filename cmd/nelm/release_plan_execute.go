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

type releasePlanExecuteConfig struct {
	action.ReleasePlanExecuteOptions

	LogColorMode string
	LogLevel     string
}

func newReleasePlanExecuteCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releasePlanExecuteConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"execute [options...] plan.json",
		"Execute previously generated plan artifact.",
		"Execute previously generated plan artifact.",
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

			if err := action.ReleasePlanExecute(ctx, cfg.ReleasePlanExecuteOptions); err != nil {
				return fmt.Errorf("plan execute: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := AddKubeConnectionFlags(cmd, &cfg.KubeConnectionOptions); err != nil {
			return fmt.Errorf("add kube connection flags: %w", err)
		}

		if err := AddResourceValidationFlags(cmd, &cfg.ResourceValidationOptions); err != nil {
			return fmt.Errorf("add resource validation flags: %w", err)
		}

		if err := AddTrackingFlags(cmd, &cfg.TrackingOptions); err != nil {
			return fmt.Errorf("add tracking flags: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.AutoRollback, "auto-rollback", false, "Automatically rollback the release on failure", cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraAnnotations, "annotations", map[string]string{}, "Add annotations to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraLabels, "labels", map[string]string{}, "Add labels to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Add annotations which will not trigger resource updates to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraRuntimeLabels, "runtime-labels", map[string]string{}, "Add labels which will not trigger resource updates to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ForceAdoption, "force-adoption", false, "Always adopt resources, even if they belong to a different Helm release", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.InstallGraphPath, "save-graph-to", "", "Save the Graphviz install graph to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.InstallReportPath, "save-report-to", "", "Save the install report to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NetworkParallelism, "network-parallelism", common.DefaultNetworkParallelism, "Limit of network-related tasks to run in parallel", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoInstallStandaloneCRDs, "no-install-crds", false, `Don't install CRDs from "crds/" directories of installed charts`, cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoRemoveManualChanges, "no-remove-manual-changes", false, "Don't remove fields added manually to the resource in the cluster if fields aren't present in the manifest", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoShowNotes, "no-notes", false, "Don't show release notes at the end of the release", cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseHistoryLimit, "release-history-limit", common.DefaultReleaseHistoryLimit, "Limit the number of releases in release history. When limit is exceeded the oldest releases are deleted. Release resources are not affected", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseInfoAnnotations, "release-info-annotations", map[string]string{}, "Add annotations to release metadata", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseLabels, "release-labels", map[string]string{}, "Add labels to the release. What kind of labels depends on the storage driver", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
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

		if err := cli.AddFlag(cmd, &cfg.RollbackGraphPath, "save-rollback-graph-to", "", "Save the Graphviz rollback graph to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ShowSubchartNotes, "show-subchart-notes", false, "Show NOTES.txt of subcharts after the release", cli.AddFlagOptions{
			Group: mainFlagGroup,
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

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "Temporary directory for execution", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.Timeout, "timeout", 0, "Fail if not finished in time", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
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
