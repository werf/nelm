package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
)

type releaseRollbackConfig struct {
	action.ReleaseRollbackOptions

	LogColorMode     string
	LogLevel         string
	ReleaseName      string
	ReleaseNamespace string
}

func newReleaseRollbackCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseRollbackConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"rollback [options...] -n namespace -r release [revision]",
		"Rollback to a previously deployed release.",
		"Rollback to a previously deployed release. Choose the last successful revision (except the very last revision), by default.",
		70,
		releaseCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
		},
		func(cmd *cobra.Command, args []string) error {
			ctx = action.SetupLogging(ctx, cfg.LogLevel, action.DefaultReleaseRollbackLogLevel, cfg.LogColorMode, false)

			if len(args) > 0 {
				var err error
				cfg.Revision, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("parse revision: %w", err)
				}
			}

			if err := action.ReleaseRollback(ctx, cfg.ReleaseName, cfg.ReleaseNamespace, cfg.ReleaseRollbackOptions); err != nil {
				return fmt.Errorf("release rollback: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.RollbackGraphPath, "save-graph-to", "", "Save the Graphviz rollback graph to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.RollbackReportPath, "save-report-to", "", "Save the rollback report to a file", cli.AddFlagOptions{
			Group: mainFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Add annotations which will not trigger resource updates to all resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeAPIServerName, "kube-api-server", "", "Kubernetes API server address", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeBurstLimit, "kube-burst-limit", action.DefaultBurstLimit, "Burst limit for requests to Kubernetes", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeCAPath, "kube-ca", "", "Path to Kubernetes API server CA file", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeConfigBase64, "kube-config-base64", "", "Pass kubeconfig file content encoded as base64", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeConfigPaths, "kube-config", []string{}, "Kubeconfig path(s). If multiple specified, their contents are merged", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: func(cmd *cobra.Command, flagName string) ([]*cli.FlagRegexExpr, error) {
				regexes := []*cli.FlagRegexExpr{cli.NewFlagRegexExpr("^KUBECONFIG$", "$KUBECONFIG")}

				if r, err := cli.GetFlagGlobalAndLocalMultiEnvVarRegexes(cmd, flagName); err != nil {
					return nil, fmt.Errorf("get local env var regexes: %w", err)
				} else {
					regexes = append(regexes, r...)
				}

				return regexes, nil
			},
			Group: kubeConnectionFlagGroup,
			Type:  cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeContext, "kube-context", "", "Kubeconfig context", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeQPSLimit, "kube-qps-limit", action.DefaultQPSLimit, "Queries Per Second limit for requests to Kubernetes", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeSkipTLSVerify, "no-verify-kube-tls", false, "Don't verify TLS certificates of Kubernetes API", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeTLSServerName, "kube-api-server-tls-name", "", "The server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.KubeToken, "kube-token", "", "The bearer token for authentication in Kubernetes API", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogColorMode, "color-mode", action.DefaultLogColorMode, "Color mode for logs. "+allowedLogColorModesHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", action.DefaultReleaseRollbackLogLevel, "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NetworkParallelism, "network-parallelism", action.DefaultNetworkParallelism, "Limit of network-related tasks to run in parallel", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.NoProgressTablePrint, "no-show-progress", false, "Don't show logs, events and real-time info about release resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ProgressTablePrintInterval, "progress-interval", action.DefaultProgressPrintInterval, "How often to print new logs, events and real-time info about release resources", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseHistoryLimit, "release-history-limit", action.DefaultReleaseHistoryLimit, "Limit the number of releases in release history. When limit is exceeded the oldest releases are deleted. Release resources are not affected", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
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

		// TODO(ilya-lesikov): restrict allowed values
		if err := cli.AddFlag(cmd, &cfg.ReleaseStorageDriver, "release-storage", "", "How releases should be stored", cli.AddFlagOptions{
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

		if err := cli.AddFlag(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 cli.FlagTypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TrackCreationTimeout, "resource-creation-timeout", 0, "Fail if resource creation tracking did not finish in time", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TrackDeletionTimeout, "resource-deletion-timeout", 0, "Fail if resource deletion tracking did not finish in time", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.TrackReadinessTimeout, "resource-readiness-timeout", 0, "Fail if resource readiness tracking did not finish in time", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
