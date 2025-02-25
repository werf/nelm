package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
)

type releaseUninstallConfig struct {
	NoDeleteHooks              bool
	DeleteReleaseNamespace     bool
	KubeAPIServerName          string
	KubeBurstLimit             int
	KubeCAPath                 string
	KubeConfigBase64           string
	KubeConfigPaths            []string
	KubeContext                string
	KubeQPSLimit               int
	KubeSkipTLSVerify          bool
	KubeTLSServerName          string
	KubeToken                  string
	LogDebug                   bool
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseName                string
	ReleaseNamespace           string
	TempDirPath                string

	releaseStorageDriver string
}

func (c *releaseUninstallConfig) ReleaseStorageDriver() action.ReleaseStorageDriver {
	return action.ReleaseStorageDriver(c.releaseStorageDriver)
}

func newReleaseUninstallCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseUninstallConfig{}

	cmd := &cobra.Command{
		Use:                   "uninstall [options...] -n namespace release",
		Short:                 "Uninstall a Helm Release from Kubernetes.",
		Long:                  "Uninstall a Helm Release from Kubernetes.",
		Args:                  cobra.ExactArgs(1),
		ValidArgsFunction:     cobra.NoFileCompletions,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ReleaseName = args[0]

			if err := action.Uninstall(ctx, action.UninstallOptions{
				DeleteHooks:                !cfg.NoDeleteHooks,
				DeleteReleaseNamespace:     cfg.DeleteReleaseNamespace,
				KubeAPIServerName:          cfg.KubeAPIServerName,
				KubeBurstLimit:             cfg.KubeBurstLimit,
				KubeCAPath:                 cfg.KubeCAPath,
				KubeConfigBase64:           cfg.KubeConfigBase64,
				KubeConfigPaths:            cfg.KubeConfigPaths,
				KubeContext:                cfg.KubeContext,
				KubeQPSLimit:               cfg.KubeQPSLimit,
				KubeSkipTLSVerify:          cfg.KubeSkipTLSVerify,
				KubeTLSServerName:          cfg.KubeTLSServerName,
				KubeToken:                  cfg.KubeToken,
				LogDebug:                   cfg.LogDebug,
				ProgressTablePrintInterval: cfg.ProgressTablePrintInterval,
				ReleaseHistoryLimit:        cfg.ReleaseHistoryLimit,
				ReleaseName:                cfg.ReleaseName,
				ReleaseNamespace:           cfg.ReleaseNamespace,
				ReleaseStorageDriver:       cfg.ReleaseStorageDriver(),
				TempDirPath:                cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("uninstall: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(cmd, &cfg.NoDeleteHooks, "no-delete-hooks", false, "Do not remove release hooks", flag.AddOptions{
			Group: mainFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.DeleteReleaseNamespace, "delete-namespace", false, "Delete the release namespace", flag.AddOptions{
			Group: mainFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeAPIServerName, "kube-api-server", "", "Kubernetes API server address", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeBurstLimit, "kube-burst-limit", action.DefaultBurstLimit, "Burst limit for requests to Kubernetes", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeCAPath, "kube-ca", "", "Path to Kubernetes API server CA file", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
			Type:                 flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeConfigBase64, "kube-config-base64", "", "Pass kubeconfig file content encoded as base64", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeConfigPaths, "kube-config", []string{}, "Kubeconfig path(s). If multiple specified, their contents are merged", flag.AddOptions{
			GetEnvVarRegexesFunc: func(cmd *cobra.Command, flagName string) ([]*flag.RegexExpr, error) {
				regexes := []*flag.RegexExpr{flag.NewRegexExpr("^KUBECONFIG$", "$KUBECONFIG")}

				if r, err := flag.GetGlobalAndLocalMultiEnvVarRegexes(cmd, flagName); err != nil {
					return nil, fmt.Errorf("get local env var regexes: %w", err)
				} else {
					regexes = append(regexes, r...)
				}

				return regexes, nil
			},
			Group: kubeConnectionFlagOptions,
			Type:  flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeContext, "kube-context", "", "Kubeconfig context", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeQPSLimit, "kube-qps-limit", action.DefaultQPSLimit, "Queries Per Second limit for requests to Kubernetes", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeSkipTLSVerify, "no-verify-kube-tls", false, "Don't verify TLS certificates of Kubernetes API", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeTLSServerName, "kube-api-server-tls-name", "", "The server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeToken, "kube-token", "", "The bearer token for authentication in Kubernetes API", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.LogDebug, "debug", false, "Show debug logs", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ProgressTablePrintInterval, "progress-interval", action.DefaultProgressPrintInterval, "How often to print new logs, events and real-time info about release resources", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                progressFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseHistoryLimit, "release-history-limit", action.DefaultReleaseHistoryLimit, "Limit the number of releases in release history. When limit is exceeded the oldest releases are deleted. Release resources are not affected", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseName, "release", "", "The release name. Must be unique within the release namespace", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagOptions,
			Required:             true,
			ShortName:            "r",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseNamespace, "namespace", "", "The release namespace. Resources with no namespace will be deployed here", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagOptions,
			Required:             true,
			ShortName:            "n",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict allowed values
		if err := flag.Add(cmd, &cfg.releaseStorageDriver, "release-storage", "", "How releases should be stored", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagOptions,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			Group: miscFlagOptions,
			Type:  flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
