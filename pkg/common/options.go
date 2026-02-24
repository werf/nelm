package common

import (
	"path/filepath"
	"time"

	"github.com/samber/lo"
)

type KubeConnectionOptions struct {
	// KubeAPIServerAddress is the Kubernetes API server address (e.g., "https://kubernetes.example.com:6443").
	KubeAPIServerAddress string
	// KubeAuthProviderConfig is the configuration map for the authentication provider in Kubernetes API.
	KubeAuthProviderConfig map[string]string
	// KubeAuthProviderName is the name of the authentication provider for Kubernetes (e.g., "gcp", "azure", "oidc").
	KubeAuthProviderName string
	// KubeBasicAuthPassword is the password for HTTP basic authentication to the Kubernetes API.
	KubeBasicAuthPassword string
	// KubeBasicAuthUsername is the username for HTTP basic authentication to the Kubernetes API.
	KubeBasicAuthUsername string
	// KubeBearerTokenData is the bearer token data for authentication in Kubernetes.
	KubeBearerTokenData string
	// KubeBearerTokenPath is the path to a file containing the bearer token for Kubernetes authentication.
	KubeBearerTokenPath string
	// KubeBurstLimit is the maximum burst limit for throttling requests to Kubernetes API.
	// Defaults to DefaultBurstLimit (100) if not set or <= 0.
	KubeBurstLimit int
	// KubeConfigBase64 is the base64-encoded kubeconfig file content.
	// Takes precedence over reading from file paths.
	KubeConfigBase64 string
	// KubeConfigPaths is a list of paths to kubeconfig files. If multiple are specified, their contents are merged.
	// Defaults to ~/.kube/config if both this and KubeConfigBase64 are empty.
	KubeConfigPaths []string
	// KubeContextCluster overrides the cluster to use from the kubeconfig for the current context.
	KubeContextCluster string
	// KubeContextCurrent specifies which kubeconfig context to use (e.g., "production", "staging").
	KubeContextCurrent string
	// KubeContextUser overrides the user to use from the kubeconfig for the current context.
	KubeContextUser string
	// KubeImpersonateGroups sets the Impersonate-Group headers when authenticating in Kubernetes.
	// Used to impersonate specific groups for authorization purposes.
	KubeImpersonateGroups []string
	// KubeImpersonateUID sets the Impersonate-Uid header when authenticating in Kubernetes.
	KubeImpersonateUID string
	// KubeImpersonateUser sets the Impersonate-User header when authenticating in Kubernetes.
	// Used to perform actions as a different user.
	KubeImpersonateUser string
	// KubeProxyURL is the proxy URL to use for all requests to the Kubernetes API (e.g., "http://proxy.example.com:8080").
	KubeProxyURL string
	// KubeQPSLimit is the Queries Per Second limit for requests to Kubernetes API.
	// Controls the rate of API requests. Defaults to DefaultQPSLimit (30) if not set or <= 0.
	KubeQPSLimit int
	// KubeRequestTimeout is the timeout duration for all requests to the Kubernetes API.
	// If 0, no timeout is applied.
	KubeRequestTimeout time.Duration
	// KubeSkipTLSVerify, when true, disables TLS certificate verification for the Kubernetes API.
	// WARNING: This makes connections insecure and should only be used for testing.
	KubeSkipTLSVerify bool
	// KubeTLSCAData is the PEM-encoded TLS CA certificate data for the Kubernetes API server.
	KubeTLSCAData string
	// KubeTLSCAPath is the path to the PEM-encoded TLS CA certificate file for the Kubernetes API server.
	KubeTLSCAPath string
	// KubeTLSClientCertData is the PEM-encoded TLS client certificate data for connecting to Kubernetes API.
	KubeTLSClientCertData string
	// KubeTLSClientCertPath is the path to the PEM-encoded TLS client certificate file for connecting to Kubernetes API.
	KubeTLSClientCertPath string
	// KubeTLSClientKeyData is the PEM-encoded TLS client key data for connecting to Kubernetes API.
	KubeTLSClientKeyData string
	// KubeTLSClientKeyPath is the path to the PEM-encoded TLS client key file for connecting to Kubernetes API.
	KubeTLSClientKeyPath string
	// KubeTLSServerName is the server name to use for Kubernetes API TLS validation.
	// Useful when the API server hostname differs from the TLS certificate's subject.
	KubeTLSServerName string
}

func (opts *KubeConnectionOptions) ApplyDefaults(homeDir string) {
	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
	}

	if opts.KubeQPSLimit <= 0 {
		opts.KubeQPSLimit = DefaultQPSLimit
	}

	if opts.KubeBurstLimit <= 0 {
		opts.KubeBurstLimit = DefaultBurstLimit
	}
}

type ChartRepoConnectionOptions struct {
	// ChartRepoBasicAuthPassword is the password for HTTP basic authentication to the chart repository.
	ChartRepoBasicAuthPassword string
	// ChartRepoBasicAuthUsername is the username for HTTP basic authentication to the chart repository.
	ChartRepoBasicAuthUsername string
	// ChartRepoCAPath is the path to the TLS CA certificate file for verifying the chart repository server.
	ChartRepoCAPath string
	// ChartRepoCertPath is the path to the TLS client certificate file for connecting to the chart repository.
	ChartRepoCertPath string
	// ChartRepoInsecure, when true, allows insecure HTTP connections to the chart repository.
	// WARNING: This disables HTTPS and should only be used for testing.
	ChartRepoInsecure bool
	// ChartRepoKeyPath is the path to the TLS client key file for connecting to the chart repository.
	ChartRepoKeyPath string
	// ChartRepoPassCreds, when true, passes repository credentials to all domains during chart operations.
	// By default, credentials are only passed to the original repository domain.
	ChartRepoPassCreds bool
	// ChartRepoRequestTimeout is the timeout duration for requests to the chart repository.
	// If 0, no timeout is applied.
	ChartRepoRequestTimeout time.Duration
	// ChartRepoSkipTLSVerify, when true, disables TLS certificate verification for the chart repository.
	// WARNING: This makes connections insecure and should only be used for testing.
	ChartRepoSkipTLSVerify bool
	// ChartRepoURL is the URL of the chart repository to use for chart lookups (e.g., "https://charts.example.com").
	ChartRepoURL string
}

func (opts *ChartRepoConnectionOptions) ApplyDefaults() {}

type ValuesOptions struct {
	// DefaultValuesDisable, when true, ignores the values.yaml file from the top-level chart.
	// Useful when you want complete control over values without chart defaults.
	DefaultValuesDisable bool
	// RootSetJSON is a list of key-value pairs in "key=json" format to set
	// arbitrary things in the global root context ("$"). This is meant to be
	// generated programmatically. Do not use it unless you know what you are doing.
	RootSetJSON []string
	// RuntimeSetJSON is a list of key-value pairs in "key=json" format to set in $.Runtime.
	// This is meant to be generated programmatically. Users should prefer ValuesSetJSON.
	// Example: ["runtime.env=dev", "runtime.timestamp=1234567890"]
	// TODO(major): get rid of it
	RuntimeSetJSON []string
	// ValuesFiles is a list of paths to additional values files to merge with chart values.
	// Files are merged in order, with later files overriding earlier ones.
	ValuesFiles []string
	// ValuesSet is a list of key-value pairs in "key=value" format to set chart values.
	// Values are parsed and may become various types (string, int, bool, etc.).
	// Example: ["image.tag=v1.2.3", "replicas=3"]
	ValuesSet []string
	// ValuesSetFile is a list of key-file pairs in "key=filepath" format.
	// The value is set to the contents of the specified file.
	// Example: ["config.yaml=/path/to/config.yaml"]
	ValuesSetFile []string
	// ValuesSetJSON is a list of key-value pairs in "key=json" format.
	// Values must be valid JSON and are parsed as such.
	// Example: ["config={\"key\":\"value\"}", "list=[1,2,3]"]
	ValuesSetJSON []string
	// ValuesSetLiteral is a list of key-value pairs in "key=value" format.
	// Values always become literal strings, even if they look like numbers or booleans.
	// Example: ["version=1.0.0", "enabled=true"] results in strings "1.0.0" and "true"
	ValuesSetLiteral []string
	// ValuesSetString is a list of key-value pairs in "key=value" format.
	// Values always become strings (no type inference).
	// Example: ["image.tag=v1.2.3", "count=5"] results in strings "v1.2.3" and "5"
	ValuesSetString []string
}

func (opts *ValuesOptions) ApplyDefaults() {}

type SecretValuesOptions struct {
	// DefaultSecretValuesDisable, when true, ignores the default secret-values.yaml file from the chart.
	// Useful when you don't want to use the chart's default encrypted values.
	DefaultSecretValuesDisable bool
	// SecretKey is the encryption/decryption key for secret values files.
	// Must be set (or available via $NELM_SECRET_KEY) to work with encrypted values.
	SecretKey string
	// SecretKeyIgnore, when true, ignores the secret key and skips decryption of secret values files.
	// Useful for operations that don't require access to secrets.
	SecretKeyIgnore bool
	// SecretValuesFiles is a list of paths to encrypted values files to decrypt and merge.
	// Files are decrypted in-memory during chart operations using the secret key.
	SecretValuesFiles []string
	// SecretWorkDir is the working directory for resolving relative paths in secret operations.
	// Defaults to the current directory if not specified.
	SecretWorkDir string
}

func (opts *SecretValuesOptions) ApplyDefaults(currentDir string) {
	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir = currentDir
	}
}

type TrackingOptions struct {
	// LegacyHelmCompatibleTracking enables Helm-compatible tracking behavior: only Jobs-hooks are tracked.
	LegacyHelmCompatibleTracking bool
	// NoFinalTracking, when true, disables final tracking of resources after the release operation.
	// Final tracking waits for all resources to reach their ready state.
	NoFinalTracking bool
	// NoPodLogs, when true, disables collection and printing of Pod logs during tracking.
	// By default, logs from failing or starting Pods are shown.
	NoPodLogs bool
	// NoProgressTablePrint, when true, disables real-time progress table display.
	// The progress table shows logs, events, and status information for release resources.
	NoProgressTablePrint bool
	// ProgressTablePrintInterval is the interval for updating the progress table display.
	// Defaults to DefaultProgressPrintInterval (5 seconds) if not set or <= 0.
	ProgressTablePrintInterval time.Duration
	// TrackCreationTimeout is the timeout duration for tracking resource creation.
	// If resource creation doesn't complete within this time, the operation fails.
	// If 0, no timeout is applied and resources are tracked indefinitely.
	TrackCreationTimeout time.Duration
	// TrackDeletionTimeout is the timeout duration for tracking resource deletion.
	// If resource deletion doesn't complete within this time, the operation fails.
	// If 0, no timeout is applied and resources are tracked indefinitely.
	TrackDeletionTimeout time.Duration
	// TrackReadinessTimeout is the timeout duration for tracking resource readiness.
	// If resources don't become ready within this time, the operation fails.
	// If 0, no timeout is applied and resources are tracked indefinitely.
	TrackReadinessTimeout time.Duration
}

func (opts *TrackingOptions) ApplyDefaults() {
	if opts.ProgressTablePrintInterval <= 0 {
		opts.ProgressTablePrintInterval = DefaultProgressPrintInterval
	}
}

type ResourceValidationOptions struct {
	// NoResourceValidation Disable resource validation.
	NoResourceValidation bool `json:"noResourceValidation"`
	// LocalResourceValidation Disable KubeConform resource validation.
	LocalResourceValidation bool `json:"localResourceValidation"`
	// ValidationKubeVersion sets specific Kubernetes version and respective schemas to use on resource validation.
	ValidationKubeVersion string `json:"validationKubeVersion"`
	// ValidationSkip Do not validate resources with specific attributes.
	ValidationSkip []string `json:"validationSkip"`
	// ValidationSchemaCacheLifetime how long the schema cache should be valid.
	ValidationSchemaCacheLifetime time.Duration `json:"validationSchemaCacheLifetime"`
	// ValidationSchemas default schema sources to validate Kubernetes resources.
	ValidationSchemas []string `json:"validationSchemas"`
	// ValidationExtraSchemas extra schema sources to validate Kubernetes resources (preferred).
	ValidationExtraSchemas []string `json:"validationExtraSchemas"`
}

func (opts *ResourceValidationOptions) ApplyDefaults() {}

type ReleaseInstallRuntimeOptions struct {
	ResourceValidationOptions

	// DefaultDeletePropagation sets the deletion propagation policy for resource deletions.
	DefaultDeletePropagation string `json:"defaultDeletePropagation"`
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources.
	// These are added during chart rendering, before resources are stored in the release.
	ExtraAnnotations map[string]string `json:"extraAnnotations"`
	// ExtraLabels are additional Kubernetes labels to add to all chart resources.
	// These are added during chart rendering, before resources are stored in the release.
	ExtraLabels map[string]string `json:"extraLabels"`
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeAnnotations map[string]string `json:"extraRuntimeAnnotations"`
	// ExtraRuntimeLabels are additional labels to add to resources at runtime.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeLabels map[string]string `json:"extraRuntimeLabels"`
	// ForceAdoption, when true, allows adopting resources that belong to a different Helm release.
	// WARNING: This can lead to conflicts if resources are managed by multiple releases.
	ForceAdoption bool `json:"forceAdoption"`
	// NoInstallStandaloneCRDs, when true, skips installation of CustomResourceDefinitions from the "crds/" directory.
	// By default, CRDs are installed first before other chart resources.
	NoInstallStandaloneCRDs bool `json:"noInstallStandaloneCRDs"`
	// NoRemoveManualChanges, when true, preserves fields manually added to resources in the cluster
	// that are not present in the chart manifests. By default, such fields are removed during updates.
	NoRemoveManualChanges bool `json:"noRemoveManualChanges"`
	// ReleaseHistoryLimit sets the maximum number of release revisions to keep in storage.
	// When exceeded, the oldest revisions are deleted. Defaults to DefaultReleaseHistoryLimit if not set or <= 0.
	// Note: Only release metadata is deleted; actual Kubernetes resources are not affected.
	ReleaseHistoryLimit int `json:"releaseHistoryLimit"`
	// ReleaseInfoAnnotations are custom annotations to add to the release metadata (stored in Secret/ConfigMap).
	// These do not affect resources but can be used for tagging releases.
	ReleaseInfoAnnotations map[string]string `json:"releaseInfoAnnotations"`
	// ReleaseLabels are labels to add to the release storage object (Secret/ConfigMap).
	// Used for filtering and organizing releases in storage.
	ReleaseLabels map[string]string `json:"releaseLabels"`
	// ReleaseStorageDriver specifies how release metadata is stored in Kubernetes.
	// Valid values: "secret" (default), "configmap", "sql".
	// Defaults to "secret" if not specified or set to "default".
	ReleaseStorageDriver string `json:"releaseStorageDriver"`
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string `json:"releaseStorageSQLConnection"`
}

type ResourceDiffOptions struct {
	DiffContextLines       int
	ShowInsignificantDiffs bool
	ShowSensitiveDiffs     bool
	ShowVerboseCRDDiffs    bool
	ShowVerboseDiffs       bool
}

func (opts *ResourceDiffOptions) ApplyDefaults() {
	if opts.DiffContextLines <= 0 {
		opts.DiffContextLines = DefaultDiffContextLines
	}
}
