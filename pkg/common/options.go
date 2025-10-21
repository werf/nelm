package common

import (
	"path/filepath"
	"time"

	"github.com/samber/lo"
)

type KubeConnectionOptions struct {
	KubeAPIServerAddress   string
	KubeAuthProviderConfig map[string]string
	KubeAuthProviderName   string
	KubeBasicAuthPassword  string
	KubeBasicAuthUsername  string
	KubeBearerTokenData    string
	KubeBearerTokenPath    string
	KubeBurstLimit         int
	KubeConfigBase64       string
	KubeConfigPaths        []string
	KubeContextCluster     string
	KubeContextCurrent     string
	KubeContextUser        string
	KubeImpersonateGroups  []string
	KubeImpersonateUID     string
	KubeImpersonateUser    string
	KubeProxyURL           string
	KubeQPSLimit           int
	KubeRequestTimeout     string
	KubeSkipTLSVerify      bool
	KubeTLSCAData          string
	KubeTLSCAPath          string
	KubeTLSClientCertData  string
	KubeTLSClientCertPath  string
	KubeTLSClientKeyData   string
	KubeTLSClientKeyPath   string
	KubeTLSServerName      string
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
	ChartRepoBasicAuthPassword string
	ChartRepoBasicAuthUsername string
	ChartRepoCAPath            string
	ChartRepoCertPath          string
	ChartRepoInsecure          bool
	ChartRepoKeyPath           string
	ChartRepoPassCreds         bool
	ChartRepoRequestTimeout    time.Duration
	ChartRepoSkipTLSVerify     bool
	ChartRepoURL               string
}

func (opts *ChartRepoConnectionOptions) ApplyDefaults() {}

type ValuesOptions struct {
	DefaultValuesDisable bool
	RuntimeSetJSON       []string
	ValuesFiles          []string
	ValuesSet            []string
	ValuesSetFile        []string
	ValuesSetJSON        []string
	ValuesSetLiteral     []string
	ValuesSetString      []string
}

func (opts *ValuesOptions) ApplyDefaults() {}

type SecretValuesOptions struct {
	DefaultSecretValuesDisable bool
	SecretKey                  string
	SecretKeyIgnore            bool
	SecretValuesFiles          []string
	SecretWorkDir              string
}

func (opts *SecretValuesOptions) ApplyDefaults(currentDir string) {
	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir = currentDir
	}
}

type TrackingOptions struct {
	NoFinalTracking            bool
	NoPodLogs                  bool
	NoProgressTablePrint       bool
	ProgressTablePrintInterval time.Duration
	TrackCreationTimeout       time.Duration
	TrackDeletionTimeout       time.Duration
	TrackReadinessTimeout      time.Duration
}

func (opts *TrackingOptions) ApplyDefaults() {
	if opts.ProgressTablePrintInterval <= 0 {
		opts.ProgressTablePrintInterval = DefaultProgressPrintInterval
	}
}
