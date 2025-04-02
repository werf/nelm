package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/action"
)

type releaseInstallConfig struct {
	AutoRollback                 bool
	ChartAppVersion              string
	ChartDirPath                 string
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	InstallGraphPath             string
	InstallGraphSave             bool
	InstallReportPath            string
	InstallReportSave            bool
	KubeAPIServerName            string
	KubeBurstLimit               int
	KubeCAPath                   string
	KubeConfigBase64             string
	KubeConfigPaths              []string
	KubeContext                  string
	KubeQPSLimit                 int
	KubeSkipTLSVerify            bool
	KubeTLSServerName            string
	KubeToken                    string
	LogColorMode                 string
	LogLevel                     string
	NetworkParallelism           int
	NoProgressTablePrint         bool
	ProgressTablePrintInterval   time.Duration
	RegistryCredentialsPath      string
	ReleaseHistoryLimit          int
	ReleaseInfoAnnotations       map[string]string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         string
	RollbackGraphPath            string
	RollbackGraphSave            bool
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SubNotes                     bool
	TempDirPath                  string
	TrackCreationTimeout         time.Duration
	TrackDeletionTimeout         time.Duration
	TrackReadinessTimeout        time.Duration
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func newReleaseInstallCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseInstallConfig{}

	cmd := cli.NewSubCommand(
		ctx,
		"install [options...] -n namespace -r release [chart-dir]",
		"Deploy a chart to Kubernetes.",
		"Deploy a chart to Kubernetes.",
		80,
		releaseCmdGroup,
		cli.SubCommandOptions{
			Args: cobra.MaximumNArgs(1),
			ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			},
		},
		func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.ReleaseInstall(ctx, cfg.ReleaseName, cfg.ReleaseNamespace, action.ReleaseInstallOptions{
				AutoRollback:                 cfg.AutoRollback,
				ChartAppVersion:              cfg.ChartAppVersion,
				ChartDirPath:                 cfg.ChartDirPath,
				ChartRepositoryInsecure:      cfg.ChartRepositoryInsecure,
				ChartRepositorySkipTLSVerify: cfg.ChartRepositorySkipTLSVerify,
				ChartRepositorySkipUpdate:    cfg.ChartRepositorySkipUpdate,
				DefaultSecretValuesDisable:   cfg.DefaultSecretValuesDisable,
				DefaultValuesDisable:         cfg.DefaultValuesDisable,
				ExtraAnnotations:             cfg.ExtraAnnotations,
				ExtraLabels:                  cfg.ExtraLabels,
				ExtraRuntimeAnnotations:      cfg.ExtraRuntimeAnnotations,
				InstallGraphPath:             cfg.InstallGraphPath,
				InstallGraphSave:             cfg.InstallGraphSave,
				InstallReportPath:            cfg.InstallReportPath,
				InstallReportSave:            cfg.InstallReportSave,
				KubeAPIServerName:            cfg.KubeAPIServerName,
				KubeBurstLimit:               cfg.KubeBurstLimit,
				KubeCAPath:                   cfg.KubeCAPath,
				KubeConfigBase64:             cfg.KubeConfigBase64,
				KubeConfigPaths:              cfg.KubeConfigPaths,
				KubeContext:                  cfg.KubeContext,
				KubeQPSLimit:                 cfg.KubeQPSLimit,
				KubeSkipTLSVerify:            cfg.KubeSkipTLSVerify,
				KubeTLSServerName:            cfg.KubeTLSServerName,
				KubeToken:                    cfg.KubeToken,
				LogColorMode:                 cfg.LogColorMode,
				LogLevel:                     cfg.LogLevel,
				NetworkParallelism:           cfg.NetworkParallelism,
				ProgressTablePrint:           !cfg.NoProgressTablePrint,
				ProgressTablePrintInterval:   cfg.ProgressTablePrintInterval,
				RegistryCredentialsPath:      cfg.RegistryCredentialsPath,
				ReleaseHistoryLimit:          cfg.ReleaseHistoryLimit,
				ReleaseInfoAnnotations:       cfg.ReleaseInfoAnnotations,
				ReleaseStorageDriver:         cfg.ReleaseStorageDriver,
				RollbackGraphPath:            cfg.RollbackGraphPath,
				RollbackGraphSave:            cfg.RollbackGraphSave,
				SecretKey:                    cfg.SecretKey,
				SecretKeyIgnore:              cfg.SecretKeyIgnore,
				SecretValuesPaths:            cfg.SecretValuesPaths,
				SubNotes:                     cfg.SubNotes,
				TempDirPath:                  cfg.TempDirPath,
				TrackCreationTimeout:         cfg.TrackCreationTimeout,
				TrackDeletionTimeout:         cfg.TrackDeletionTimeout,
				TrackReadinessTimeout:        cfg.TrackReadinessTimeout,
				ValuesFileSets:               cfg.ValuesFileSets,
				ValuesFilesPaths:             cfg.ValuesFilesPaths,
				ValuesSets:                   cfg.ValuesSets,
				ValuesStringSets:             cfg.ValuesStringSets,
			}); err != nil {
				return fmt.Errorf("install: %w", err)
			}

			return nil
		},
	)

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := cli.AddFlag(cmd, &cfg.AutoRollback, "auto-rollback", false, "Automatically rollback the release on failure", cli.AddFlagOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartAppVersion, "app-version", "", "Set appVersion of Chart.yaml", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepositoryInsecure, "insecure-chart-repos", false, "Allow insecure HTTP connections to chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepositorySkipTLSVerify, "no-verify-chart-repos-tls", false, "Don't verify TLS certificates of chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ChartRepositorySkipUpdate, "no-update-chart-repos", false, "Don't update chart repositories index", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.DefaultSecretValuesDisable, "no-default-secret-values", false, "Ignore secret-values.yaml of the top-level chart", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.DefaultValuesDisable, "no-default-values", false, "Ignore values.yaml of the top-level chart", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
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

		if err := cli.AddFlag(cmd, &cfg.LogLevel, "log-level", action.DefaultReleaseInstallLogLevel, "Set log level. "+allowedLogLevelsHelp(), cli.AddFlagOptions{
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

		if err := cli.AddFlag(cmd, &cfg.RegistryCredentialsPath, "oci-chart-repos-creds", action.DefaultRegistryCredentialsPath, "Credentials to access OCI chart repositories", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseHistoryLimit, "release-history-limit", action.DefaultReleaseHistoryLimit, "Limit the number of releases in release history. When limit is exceeded the oldest releases are deleted. Release resources are not affected", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ReleaseInfoAnnotations, "release-info-annotations", map[string]string{}, "Add annotations to release metadata", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagLocalMultiEnvVarRegexes,
			Group:                mainFlagGroup,
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

		if err := cli.AddFlag(cmd, &cfg.SecretKey, "secret-key", "", "Secret key", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretKeyIgnore, "no-decrypt-secrets", false, "Do not decrypt secrets and secret values, pass them as is", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SecretValuesPaths, "secret-values", []string{}, "Secret values files paths", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.SubNotes, "show-subchart-notes", false, "Show NOTES.txt of subcharts after the release", cli.AddFlagOptions{
			Group: mainFlagGroup,
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

		if err := cli.AddFlag(cmd, &cfg.ValuesFileSets, "set-file", []string{}, "Set new values, where the key is the value path and the value is the path to the file with the value content", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ValuesFilesPaths, "values", []string{}, "Additional values files", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
			Type:                 cli.FlagTypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ValuesSets, "set", []string{}, "Set new values, where the key is the value path and the value is the value", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := cli.AddFlag(cmd, &cfg.ValuesStringSets, "set-string", []string{}, "Set new values, where the key is the value path and the value is the value. The value will always be a string", cli.AddFlagOptions{
			GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
