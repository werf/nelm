package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/common"
)

func AddKubeConnectionFlags(cmd *cobra.Command, cfg common.KubeConnectionOptions) error {
	if err := cli.AddFlag(cmd, &cfg.KubeAPIServerAddress, "kube-api-server", "", "Kubernetes API server address", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBurstLimit, "kube-burst-limit", common.DefaultBurstLimit, "Burst limit for requests to Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                performanceFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSCAPath, "kube-ca", "", "Path to Kubernetes API server CA file", cli.AddFlagOptions{
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

	if err := cli.AddFlag(cmd, &cfg.KubeContextCurrent, "kube-context", "", "Kubeconfig context", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeQPSLimit, "kube-qps-limit", common.DefaultQPSLimit, "Queries Per Second limit for requests to Kubernetes", cli.AddFlagOptions{
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

	if err := cli.AddFlag(cmd, &cfg.KubeBearerTokenData, "kube-token", "", "The bearer token for authentication in Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddChartRepoConnectionFlags(cmd *cobra.Command, cfg common.ChartRepoConnectionOptions) error {
	if err := cli.AddFlag(cmd, &cfg.ChartRepoInsecure, "insecure-chart-repos", false, "Allow insecure HTTP connections to chart repositories", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoSkipTLSVerify, "no-verify-chart-repos-tls", false, "Don't verify TLS certificates of chart repositories", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddValuesFlags(cmd *cobra.Command, cfg common.ValuesOptions) error {
	if err := cli.AddFlag(cmd, &cfg.DefaultValuesDisable, "no-default-values", false, "Ignore values.yaml of the top-level chart", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValuesFiles, "values", []string{}, "Additional values files", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	// TODO(v2): revise all flags in nelm/werf to make sure they are all parsed as it happens in
	// Helm (see https://github.com/werf/nelm/issues/337)
	if err := cli.AddFlag(cmd, &cfg.ValuesSet, "set", []string{}, "Set new values, where the key is the value path and the value is the value", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValuesSetFile, "set-file", []string{}, "Set new values, where the key is the value path and the value is the path to the file with the value content", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValuesSetString, "set-string", []string{}, "Set new values, where the key is the value path and the value is the value. The value will always be a string", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddSecretValuesFlags(cmd *cobra.Command, cfg common.SecretValuesOptions) error {
	if err := cli.AddFlag(cmd, &cfg.DefaultSecretValuesDisable, "no-default-secret-values", false, "Ignore secret-values.yaml of the top-level chart", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                secretFlagGroup,
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

	if err := cli.AddFlag(cmd, &cfg.SecretValuesFiles, "secret-values", []string{}, "Secret values files paths", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                secretFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddTrackingFlags(cmd *cobra.Command, cfg *common.TrackingOptions) error {
	if err := cli.AddFlag(cmd, &cfg.NoFinalTracking, "no-final-tracking", false, "By default disable tracking operations that have no create/update/delete resource operations after them, which are most tracking operations, to speed up the release", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                progressFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.NoPodLogs, "no-pod-logs", false, "Disable Pod logs collection and printing", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                progressFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.NoProgressTablePrint, "no-show-progress", false, "Don't show logs, events and real-time info about release resources", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                progressFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ProgressTablePrintInterval, "progress-interval", common.DefaultProgressPrintInterval, "How often to print new logs, events and real-time info about release resources", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                progressFlagGroup,
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
