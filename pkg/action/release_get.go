package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseGetOutputFormat = YamlOutputFormat
	DefaultReleaseGetLogLevel     = log.ErrorLevel
)

type ReleaseGetOptions struct {
	KubeAPIServerAddress        string
	KubeAuthProviderConfig      map[string]string
	KubeAuthProviderName        string
	KubeBasicAuthPassword       string
	KubeBasicAuthUsername       string
	KubeBearerTokenData         string
	KubeBearerTokenPath         string
	KubeBurstLimit              int
	KubeConfigBase64            string
	KubeConfigPaths             []string
	KubeContextCluster          string
	KubeContextCurrent          string
	KubeContextUser             string
	KubeImpersonateGroups       []string
	KubeImpersonateUID          string
	KubeImpersonateUser         string
	KubeProxyURL                string
	KubeQPSLimit                int
	KubeRequestTimeout          string
	KubeSkipTLSVerify           bool
	KubeTLSCAData               string
	KubeTLSCAPath               string
	KubeTLSClientCertData       string
	KubeTLSClientCertPath       string
	KubeTLSClientKeyData        string
	KubeTLSClientKeyPath        string
	KubeTLSServerName           string
	NetworkParallelism          int
	OutputFormat                string
	OutputNoPrint               bool
	PrintValues                 bool
	ReleaseStorageDriver        string
	ReleaseStorageSQLConnection string
	Revision                    int
	TempDirPath                 string
}

func ReleaseGet(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseGetOptions) (*ReleaseGetResultV1, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseGetOptionsDefaults(opts, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build release get options: %w", err)
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		APIServerAddress:   opts.KubeAPIServerAddress,
		AuthProviderConfig: opts.KubeAuthProviderConfig,
		AuthProviderName:   opts.KubeAuthProviderName,
		BasicAuthPassword:  opts.KubeBasicAuthPassword,
		BasicAuthUsername:  opts.KubeBasicAuthUsername,
		BearerTokenData:    opts.KubeBearerTokenData,
		BearerTokenPath:    opts.KubeBearerTokenPath,
		BurstLimit:         opts.KubeBurstLimit,
		ContextCluster:     opts.KubeContextCluster,
		ContextCurrent:     opts.KubeContextCurrent,
		ContextNamespace:   releaseNamespace, // TODO: unset it everywhere
		ContextUser:        opts.KubeContextUser,
		ImpersonateGroups:  opts.KubeImpersonateGroups,
		ImpersonateUID:     opts.KubeImpersonateUID,
		ImpersonateUser:    opts.KubeImpersonateUser,
		KubeConfigBase64:   opts.KubeConfigBase64,
		ProxyURL:           opts.KubeProxyURL,
		QPSLimit:           opts.KubeQPSLimit,
		RequestTimeout:     opts.KubeRequestTimeout,
		SkipTLSVerify:      opts.KubeSkipTLSVerify,
		TLSCAData:          opts.KubeTLSCAData,
		TLSCAPath:          opts.KubeTLSCAPath,
		TLSClientCertData:  opts.KubeTLSClientCertData,
		TLSClientCertPath:  opts.KubeTLSClientCertPath,
		TLSClientKeyData:   opts.KubeTLSClientKeyData,
		TLSClientKeyPath:   opts.KubeTLSClientKeyPath,
		TLSServerName:      opts.KubeTLSServerName,
	})
	if err != nil {
		return nil, fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	loader.NoChartLockWarning = ""

	log.Default.Debug(ctx, "Build release history")

	history, err := release.BuildHistory(releaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return nil, fmt.Errorf("build release history: %w", err)
	}

	releases := history.Releases()
	if len(releases) == 0 {
		return nil, &ReleaseNotFoundError{
			ReleaseName:      releaseName,
			ReleaseNamespace: releaseNamespace,
		}
	}

	var rel *helmrelease.Release
	if opts.Revision == 0 {
		rel = lo.LastOrEmpty(releases)
	} else {
		var revisionFound bool

		rel, revisionFound = history.FindRevision(opts.Revision)
		if !revisionFound {
			return nil, &ReleaseRevisionNotFoundError{
				ReleaseName:      releaseName,
				ReleaseNamespace: releaseNamespace,
				Revision:         opts.Revision,
			}
		}
	}

	values, err := chartutil.CoalesceValues(rel.Chart, rel.Config)
	if err != nil {
		return nil, fmt.Errorf("coalesce release values: %w", err)
	}

	result := &ReleaseGetResultV1{
		APIVersion: "v1",
		Release: &ReleaseGetResultRelease{
			Name:      rel.Name,
			Namespace: rel.Namespace,
			Revision:  rel.Version,
			Status:    rel.Info.Status,
			DeployedAt: &ReleaseGetResultDeployedAt{
				Human: time.Time{}.String(),
				Unix:  int(time.Time{}.Unix()),
			},
			Annotations:   rel.Info.Annotations,
			StorageLabels: rel.Labels,
		},
		Chart: &ReleaseGetResultChart{
			Name:       rel.Chart.Name(),
			Version:    rel.Chart.Metadata.Version,
			AppVersion: rel.Chart.Metadata.AppVersion,
		},
		Notes:  rel.Info.Notes,
		Values: values,
	}

	resSpecs, err := release.ReleaseToResourceSpecs(rel, releaseNamespace)
	if err != nil {
		return nil, fmt.Errorf("convert release to resource specs: %w", err)
	}

	for _, res := range resSpecs {
		if spec.IsHook(res.Annotations) {
			result.Hooks = append(result.Hooks, res.Unstruct.Object)
		} else {
			result.Resources = append(result.Resources, res.Unstruct.Object)
		}
	}

	if opts.OutputNoPrint {
		return result, nil
	}

	var resultMessage string

	savedValues := result.Values
	if !opts.PrintValues {
		result.Values = nil
	}

	switch opts.OutputFormat {
	case JSONOutputFormat:
		b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
		if err != nil {
			return nil, fmt.Errorf("marshal result to json: %w", err)
		}

		resultMessage = string(b)
	case YamlOutputFormat:
		b, err := yaml.MarshalContext(ctx, result, yaml.UseLiteralStyleIfMultiline(true))
		if err != nil {
			return nil, fmt.Errorf("marshal result to yaml: %w", err)
		}

		resultMessage = string(b)
	default:
		return nil, fmt.Errorf("unknown output format %q", opts.OutputFormat)
	}

	if !opts.PrintValues {
		result.Values = savedValues
	}

	var colorLevel color.Level
	if color.Enable {
		colorLevel = color.TermColorLevel()
	}

	if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, opts.OutputFormat, colorLevel); err != nil {
		return nil, fmt.Errorf("write result to output: %w", err)
	}

	return result, nil
}

func applyReleaseGetOptionsDefaults(opts ReleaseGetOptions, homeDir string) (ReleaseGetOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseGetOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = DefaultNetworkParallelism
	}

	if opts.KubeQPSLimit <= 0 {
		opts.KubeQPSLimit = DefaultQPSLimit
	}

	if opts.KubeBurstLimit <= 0 {
		opts.KubeBurstLimit = DefaultBurstLimit
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = DefaultReleaseGetOutputFormat
	}

	return opts, nil
}

type ReleaseGetResultV1 struct {
	APIVersion string                   `json:"apiVersion"`
	Release    *ReleaseGetResultRelease `json:"release"`
	Chart      *ReleaseGetResultChart   `json:"chart"`
	Notes      string                   `json:"notes,omitempty"`
	Values     map[string]interface{}   `json:"values,omitempty"`
	// TODO(v2): Join Hooks and Resources together as ResourceSpecs?
	Hooks     []map[string]interface{} `json:"hooks,omitempty"`
	Resources []map[string]interface{} `json:"resources,omitempty"`
}

type ReleaseGetResultRelease struct {
	Name          string                      `json:"name"`
	Namespace     string                      `json:"namespace"`
	Revision      int                         `json:"revision"`
	Status        helmrelease.Status          `json:"status"`
	DeployedAt    *ReleaseGetResultDeployedAt `json:"deployedAt"`
	Annotations   map[string]string           `json:"annotations"`
	StorageLabels map[string]string           `json:"storageLabels"`
}

// TODO(v2): get rid
type ReleaseGetResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseGetResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}
