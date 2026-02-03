package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/werf/common-go/pkg/cli"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

func AddKubeConnectionFlags(cmd *cobra.Command, cfg *common.KubeConnectionOptions) error {
	if err := cli.AddFlag(cmd, &cfg.KubeAPIServerAddress, "kube-api-server", "", "Kubernetes API server address", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeAuthProviderConfig, "kube-auth-provider-config", nil, "Auth provider config for authentication in Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeAuthProviderName, "kube-auth-provider", "", "Auth provider name for authentication in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBasicAuthPassword, "kube-auth-password", "", "Basic auth password for Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBasicAuthUsername, "kube-auth-username", "", "Basic auth username for Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBearerTokenData, "kube-token", "", "Bearer token for authentication in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBearerTokenPath, "kube-token-path", "", "Path to file with bearer token for authentication in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeBurstLimit, "kube-burst-limit", common.DefaultBurstLimit, "Burst limit for requests to Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                performanceFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeConfigBase64, "kube-config-base64", "", "Pass Kubeconfig file content encoded as base64", cli.AddFlagOptions{
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

	if err := cli.AddFlag(cmd, &cfg.KubeContextCluster, "kube-context-cluster", "", "Use cluster from Kubeconfig for current context", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeContextCurrent, "kube-context", "", "Use specified Kubeconfig context", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeContextUser, "kube-context-user", "", "Use user from Kubeconfig for current context", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeImpersonateGroups, "kube-impersonate-group", nil, "Sets Impersonate-Group headers when authenticating in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeImpersonateUID, "kube-impersonate-uid", "", "Sets Impersonate-Uid header when authenticating in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeImpersonateUser, "kube-impersonate-user", "", "Sets Impersonate-User header when authenticating in Kubernetes", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeProxyURL, "kube-proxy-url", "", "Proxy URL to use for proxying all requests to Kubernetes API", cli.AddFlagOptions{
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

	if err := cli.AddFlag(cmd, &cfg.KubeRequestTimeout, "kube-request-timeout", 0, "Timeout for all requests to Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeSkipTLSVerify, "no-verify-kube-tls", false, "Don't verify TLS certificates of Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSCAData, "kube-ca-data", "", "Pass Kubernetes API server TLS CA data", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSCAPath, "kube-ca", "", "Path to Kubernetes API server TLS CA file", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSClientCertData, "kube-cert-data", "", "Pass PEM-encoded TLS client cert for connecting to Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSClientCertPath, "kube-cert", "", "Path to PEM-encoded TLS client cert for connecting to Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSClientKeyData, "kube-key-data", "", "Pass PEM-encoded TLS client key for connecting to Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSClientKeyPath, "kube-key", "", "Path to PEM-encoded TLS client key for connecting to Kubernetes API", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.KubeTLSServerName, "kube-api-server-tls-name", "", "Server name for Kubernetes API TLS validation, if different from the hostname of Kubernetes API server", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                kubeConnectionFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddChartRepoConnectionFlags(cmd *cobra.Command, cfg *common.ChartRepoConnectionOptions) error {
	if err := cli.AddFlag(cmd, &cfg.ChartRepoBasicAuthPassword, "chart-repo-basic-password", "", "Basic auth password to authenticate in chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoBasicAuthUsername, "chart-repo-basic-username", "", "Basic auth username to authenticate in chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoCAPath, "chart-repo-ca", "", "Path to TLS CA file for connecting to chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoCertPath, "chart-repo-cert", "", "Path to TLS client cert file for connecting to chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoInsecure, "insecure-chart-repos", false, "Allow insecure HTTP connections to chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoKeyPath, "chart-repo-key", "", "Path to TLS client key file for connecting to chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
		Type:                 cli.FlagTypeFile,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoPassCreds, "chart-repo-pass-creds", false, "Allow sending chart repository credentials to domains different from the chart repository domain when downloading charts", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoRequestTimeout, "chart-repo-request-timeout", 0, "Set timeout for all requests to chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoSkipTLSVerify, "no-verify-chart-repos-tls", false, "Don't verify TLS certificates of chart repository", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ChartRepoURL, "chart-repo-url", "", "Set URL of chart repo to be used to look for chart", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                chartRepoFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddResourceValidationFlags(cmd *cobra.Command, cfg *common.ResourceValidationOptions) error {
	if !featgate.FeatGateResourceValidation.Enabled() {
		return nil
	}

	if err := cli.AddFlag(cmd, &cfg.NoResourceValidation, "no-resource-validation", false, "Disable resource validation", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                resourceValidationGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.LocalResourceValidation, "local-resource-validation", false, "Do not use external json schema sources", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                resourceValidationGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValidationKubeVersion, "resource-validation-kube-version", common.DefaultResourceValidationKubeVersion, "Kubernetes schemas version to use during resource validation", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                resourceValidationGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValidationSkip, "resource-validation-skip", []string{}, "Skip resource validation for resources with specified attributes. Format: key1=value1,key2=value2. Supported keys: group, version, kind, name, namespace. Example: kind=Deployment,name=my-app", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                resourceValidationGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValidationSchemas, "resource-validation-schema", common.DefaultResourceValidationSchema, "Default json schema sources to validate resources. Must be a valid go template defining a http(s) URL, or an absolute path on local file system", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
		Group:                resourceValidationGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValidationExtraSchemas, "resource-validation-extra-schema", []string{}, "Extra json schema sources to validate resources (preferred over default sources). Must be a valid go template defining a http(s) URL, or an absolute path on local file system", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalMultiEnvVarRegexes,
		Group:                resourceValidationGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValidationSchemaCacheLifetime, "resource-validation-cache-lifetime", common.DefaultResourceValidationCacheLifetime, "How long local schema cache will be valid", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                resourceValidationGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddValuesFlags(cmd *cobra.Command, cfg *common.ValuesOptions) error {
	if err := cli.AddFlag(cmd, &cfg.DefaultValuesDisable, "no-default-values", false, "Ignore values.yaml of the top-level chart", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.RuntimeSetJSON, "set-runtime-json", []string{}, "Set new keys in $.Runtime, where the key is the value path and the value is JSON. This is meant to be generated inside the program, so use --set-json instead, unless you know what you are doing", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
		NoSplitOnCommas:      true,
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

	if err := cli.AddFlag(cmd, &cfg.ValuesSetJSON, "set-json", []string{}, "Set new values, where the key is the value path and the value is JSON", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValuesSetLiteral, "set-literal", []string{}, "Set new values, where the key is the value path and the value is the value. The value will always become a literal string", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
		NoSplitOnCommas:      true,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	if err := cli.AddFlag(cmd, &cfg.ValuesSetString, "set-string", []string{}, "Set new values, where the key is the value path and the value is the value. The value will always become a string", cli.AddFlagOptions{
		GetEnvVarRegexesFunc: cli.GetFlagGlobalAndLocalEnvVarRegexes,
		Group:                valuesFlagGroup,
	}); err != nil {
		return fmt.Errorf("add flag: %w", err)
	}

	return nil
}

func AddSecretValuesFlags(cmd *cobra.Command, cfg *common.SecretValuesOptions) error {
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
