package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type releaseGetConfig struct {
	KubeAPIServerName  string
	KubeBurstLimit     int
	KubeCAPath         string
	KubeConfigBase64   string
	KubeConfigPaths    []string
	KubeContext        string
	KubeQPSLimit       int
	KubeSkipTLSVerify  bool
	KubeTLSServerName  string
	KubeToken          string
	LogDebug           bool
	NetworkParallelism int
	ReleaseName        string
	ReleaseNamespace   string
	Revision           int
	TempDirPath        string

	logLevel             string
	outputFormat         string
	releaseStorageDriver string
}

func (c *releaseGetConfig) ReleaseStorageDriver() action.ReleaseStorageDriver {
	return action.ReleaseStorageDriver(c.releaseStorageDriver)
}

func (c *releaseGetConfig) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func (c *releaseGetConfig) OutputFormat() common.OutputFormat {
	return common.OutputFormat(c.outputFormat)
}

func newReleaseGetCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &releaseGetConfig{}

	cmd := &cobra.Command{
		Use:   "get [options...] -n namespace -r release [revision]",
		Short: "Get information about a deployed release.",
		Long:  "Get information about a deployed release.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				var err error
				cfg.Revision, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid revision: %s", args[0])
				}
			}

			if _, err := action.Get(ctx, action.GetOptions{
				KubeAPIServerName:    cfg.KubeAPIServerName,
				KubeBurstLimit:       cfg.KubeBurstLimit,
				KubeCAPath:           cfg.KubeCAPath,
				KubeConfigBase64:     cfg.KubeConfigBase64,
				KubeConfigPaths:      cfg.KubeConfigPaths,
				KubeContext:          cfg.KubeContext,
				KubeQPSLimit:         cfg.KubeQPSLimit,
				KubeSkipTLSVerify:    cfg.KubeSkipTLSVerify,
				KubeTLSServerName:    cfg.KubeTLSServerName,
				KubeToken:            cfg.KubeToken,
				LogLevel:             cfg.LogLevel(),
				NetworkParallelism:   cfg.NetworkParallelism,
				OutputFormat:         cfg.OutputFormat(),
				ReleaseName:          cfg.ReleaseName,
				ReleaseNamespace:     cfg.ReleaseNamespace,
				ReleaseStorageDriver: cfg.ReleaseStorageDriver(),
				Revision:             cfg.Revision,
				TempDirPath:          cfg.TempDirPath,
			}); err != nil {
				return fmt.Errorf("get: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(cmd, &cfg.KubeAPIServerName, "kube-api-server", "", "Kubernetes API server address", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeBurstLimit, "kube-burst-limit", action.DefaultBurstLimit, "Burst limit for requests to Kubernetes", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeCAPath, "kube-ca", "", "Path to Kubernetes API server CA file", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
			Type:                 flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeConfigBase64, "kube-config-base64", "", "Pass kubeconfig file content encoded as base64", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
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
			Group: kubeConnectionFlagGroup,
			Type:  flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeContext, "kube-context", "", "Kubeconfig context", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeQPSLimit, "kube-qps-limit", action.DefaultQPSLimit, "Queries Per Second limit for requests to Kubernetes", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeSkipTLSVerify, "no-verify-kube-tls", false, "Don't verify TLS certificates of Kubernetes API", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeTLSServerName, "kube-api-server-tls-name", "", "The server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeToken, "kube-token", "", "The bearer token for authentication in Kubernetes API", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                kubeConnectionFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// FIXME(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(log.InfoLevel), "Set log level", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.NetworkParallelism, "network-parallelism", action.DefaultNetworkParallelism, "Limit of network-related tasks to run in parallel", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                performanceFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict values
		if err := flag.Add(cmd, &cfg.outputFormat, "output-format", string(action.DefaultGetOutputFormat), "Result output format", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseName, "release", "", "The release name. Must be unique within the release namespace", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "r",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseNamespace, "namespace", "", "The release namespace. Resources with no namespace will be deployed here", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			Required:             true,
			ShortName:            "n",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict allowed values
		if err := flag.Add(cmd, &cfg.releaseStorageDriver, "release-storage", "", "How releases should be stored", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			Group: miscFlagGroup,
			Type:  flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
