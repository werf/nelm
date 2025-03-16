package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/flag"
	"github.com/werf/nelm/pkg/action"
	"github.com/werf/nelm/pkg/log"
)

type chartLintConfig struct {
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
	KubeVersion                  string
	LogDebug                     bool
	NetworkParallelism           int
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	Remote                       bool
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string

	logColorMode         string
	logLevel             string
	releaseStorageDriver string
}

func (c *chartLintConfig) ReleaseStorageDriver() action.ReleaseStorageDriver {
	return action.ReleaseStorageDriver(c.releaseStorageDriver)
}

func (c *chartLintConfig) LogColorMode() action.LogColorMode {
	return action.LogColorMode(c.logColorMode)
}

func (c *chartLintConfig) LogLevel() log.Level {
	return log.Level(c.logLevel)
}

func newChartLintCommand(ctx context.Context, afterAllCommandsBuiltFuncs map[*cobra.Command]func(cmd *cobra.Command) error) *cobra.Command {
	cfg := &chartLintConfig{}

	cmd := &cobra.Command{
		Use:   "lint [options...] [chart-dir]",
		Short: "Lint a chart.",
		Long:  "Lint a chart.",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cfg.ChartDirPath = args[0]
			}

			if err := action.ChartLint(ctx, action.ChartLintOptions{
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
				Local:                        !cfg.Remote,
				LocalKubeVersion:             cfg.KubeVersion,
				LogColorMode:                 cfg.LogColorMode(),
				LogLevel:                     cfg.LogLevel(),
				NetworkParallelism:           cfg.NetworkParallelism,
				RegistryCredentialsPath:      cfg.RegistryCredentialsPath,
				ReleaseName:                  cfg.ReleaseName,
				ReleaseNamespace:             cfg.ReleaseNamespace,
				ReleaseStorageDriver:         cfg.ReleaseStorageDriver(),
				SecretKey:                    cfg.SecretKey,
				SecretKeyIgnore:              cfg.SecretKeyIgnore,
				SecretValuesPaths:            cfg.SecretValuesPaths,
				TempDirPath:                  cfg.TempDirPath,
				ValuesFileSets:               cfg.ValuesFileSets,
				ValuesFilesPaths:             cfg.ValuesFilesPaths,
				ValuesSets:                   cfg.ValuesSets,
				ValuesStringSets:             cfg.ValuesStringSets,
			}); err != nil {
				return fmt.Errorf("chart lint: %w", err)
			}

			return nil
		},
	}

	afterAllCommandsBuiltFuncs[cmd] = func(cmd *cobra.Command) error {
		if err := flag.Add(cmd, &cfg.ChartAppVersion, "app-version", "", "Set appVersion of Chart.yaml", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ChartRepositoryInsecure, "insecure-chart-repos", false, "Allow insecure HTTP connections to chart repositories", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ChartRepositorySkipTLSVerify, "no-verify-chart-repos-tls", false, "Don't verify TLS certificates of chart repositories", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ChartRepositorySkipUpdate, "no-update-chart-repos", false, "Don't update chart repositories index", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.DefaultSecretValuesDisable, "no-default-secret-values", false, "Ignore secret-values.yaml of the top-level chart", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.DefaultValuesDisable, "no-default-values", false, "Ignore values.yaml of the top-level chart", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ExtraAnnotations, "annotations", map[string]string{}, "Add annotations to all resources", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ExtraLabels, "labels", map[string]string{}, "Add labels to all resources", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ExtraRuntimeAnnotations, "runtime-annotations", map[string]string{}, "Add annotations which will not trigger resource updates to all resources", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalMultiEnvVarRegexes,
			Group:                patchFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

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

		if err := flag.Add(cmd, &cfg.Remote, "remote", false, "Allow cluster access for additional checks", flag.AddOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.KubeVersion, "kube-version", action.DefaultLocalKubeVersion, "Kubernetes version stub for non-remote mode", flag.AddOptions{
			Group: mainFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.logColorMode, "color-mode", string(action.DefaultLogColorMode), "Color mode for logs. "+allowedLogColorModesHelp(), flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.logLevel, "log-level", string(action.DefaultChartLintLogLevel), "Set log level. "+allowedLogLevelsHelp(), flag.AddOptions{
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

		if err := flag.Add(cmd, &cfg.RegistryCredentialsPath, "oci-chart-repos-creds", action.DefaultRegistryCredentialsPath, "Credentials to access OCI chart repositories", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                chartRepoFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseName, "release", action.StubReleaseName, "The release name. Must be unique within the release namespace", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			ShortName:            "r",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ReleaseNamespace, "namespace", action.StubReleaseNamespace, "The release namespace. Resources with no namespace will be deployed here", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                mainFlagGroup,
			ShortName:            "n",
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		// TODO(ilya-lesikov): restrict allowed values
		if err := flag.Add(cmd, &cfg.releaseStorageDriver, "release-storage", "", "How releases should be stored", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.SecretKey, "secret-key", "", "Secret key", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.SecretKeyIgnore, "no-decrypt-secrets", false, "Do not decrypt secrets and secret values, pass them as is", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.SecretValuesPaths, "secret-values", []string{}, "Secret values files paths", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                secretFlagGroup,
			Type:                 flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.TempDirPath, "temp-dir", "", "The directory for temporary files. By default, create a new directory in the default system directory for temporary files", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalEnvVarRegexes,
			Group:                miscFlagGroup,
			Type:                 flag.TypeDir,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ValuesFileSets, "set-file", []string{}, "Set new values, where the key is the value path and the value is the path to the file with the value content", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ValuesFilesPaths, "values", []string{}, "Additional values files", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
			Type:                 flag.TypeFile,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ValuesSets, "set", []string{}, "Set new values, where the key is the value path and the value is the value", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		if err := flag.Add(cmd, &cfg.ValuesStringSets, "set-string", []string{}, "Set new values, where the key is the value path and the value is the value. The value will always be a string", flag.AddOptions{
			GetEnvVarRegexesFunc: flag.GetGlobalAndLocalEnvVarRegexes,
			Group:                valuesFlagGroup,
		}); err != nil {
			return fmt.Errorf("add flag: %w", err)
		}

		return nil
	}

	return cmd
}
