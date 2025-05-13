package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/3p-helm/pkg/chart/loader"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
)

const (
	DefaultReleaseGetOutputFormat = YamlOutputFormat
	DefaultReleaseGetLogLevel     = ErrorLogLevel
)

type ReleaseGetOptions struct {
	KubeAPIServerName    string
	KubeBurstLimit       int
	KubeCAPath           string
	KubeConfigBase64     string
	KubeConfigPaths      []string
	KubeContext          string
	KubeQPSLimit         int
	KubeSkipTLSVerify    bool
	KubeTLSServerName    string
	KubeToken            string
	NetworkParallelism   int
	OutputFormat         string
	OutputNoPrint        bool
	PrintValues          bool
	ReleaseStorageDriver string
	Revision             int
	SQLConnectionString  string
	TempDirPath          string
}

func ReleaseGet(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseGetOptions) (*ReleaseGetResultV1, error) {
	actionLock.Lock()
	defer actionLock.Unlock()

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

	// TODO(ilya-lesikov): some options are not propagated from cli/actions
	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		BurstLimit:            opts.KubeBurstLimit,
		CertificateAuthority:  opts.KubeCAPath,
		CurrentContext:        opts.KubeContext,
		InsecureSkipTLSVerify: opts.KubeSkipTLSVerify,
		KubeConfigBase64:      opts.KubeConfigBase64,
		Namespace:             releaseNamespace,
		QPSLimit:              opts.KubeQPSLimit,
		Server:                opts.KubeAPIServerName,
		TLSServerName:         opts.KubeTLSServerName,
		Token:                 opts.KubeToken,
	})
	if err != nil {
		return nil, fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(
		ctx,
		releaseNamespace,
		opts.ReleaseStorageDriver,
		release.ReleaseStorageOptions{
			StaticClient:        clientFactory.Static().(*kubernetes.Clientset),
			SQLConnectionString: opts.SQLConnectionString,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	secrets.DisableSecrets = true
	loader.NoChartLockWarning = ""

	history, err := release.NewHistory(
		releaseName,
		releaseNamespace,
		releaseStorage,
		release.HistoryOptions{
			Mapper:          clientFactory.Mapper(),
			DiscoveryClient: clientFactory.Discovery(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("construct release history: %w", err)
	}

	if history.Empty() {
		return nil, &ReleaseNotFoundError{
			ReleaseName:      releaseName,
			ReleaseNamespace: releaseNamespace,
		}
	}

	var (
		release *release.Release
	)
	if opts.Revision == 0 {
		release, _, err = history.LastRelease()
		if err != nil {
			return nil, fmt.Errorf("get last release: %w", err)
		}
	} else {
		var revisionFound bool
		release, revisionFound, err = history.Release(opts.Revision)
		if err != nil {
			return nil, fmt.Errorf("get release revision %d: %w", opts.Revision, err)
		} else if !revisionFound {
			return nil, &ReleaseRevisionNotFoundError{
				ReleaseName:      releaseName,
				ReleaseNamespace: releaseNamespace,
				Revision:         opts.Revision,
			}
		}
	}

	result := &ReleaseGetResultV1{
		ApiVersion: ReleaseGetResultApiVersionV1,
		Release: &ReleaseGetResultRelease{
			Name:      release.Name(),
			Namespace: release.Namespace(),
			Revision:  release.Revision(),
			Status:    release.Status(),
			DeployedAt: &ReleaseGetResultDeployedAt{
				Human: release.LastDeployed().String(),
				Unix:  int(release.LastDeployed().Unix()),
			},
			Annotations: release.InfoAnnotations(),
		},
		Chart: &ReleaseGetResultChart{
			Name:       release.ChartName(),
			Version:    release.ChartVersion(),
			AppVersion: release.AppVersion(),
		},
		Notes:  release.Notes(),
		Values: release.Values(),
	}

	for _, hook := range release.HookResources() {
		result.Hooks = append(result.Hooks, hook.Unstructured().Object)
	}

	for _, resource := range release.GeneralResources() {
		result.Resources = append(result.Resources, resource.Unstructured().Object)
	}

	if !opts.OutputNoPrint {
		var resultMessage string

		savedValues := result.Values
		if !opts.PrintValues {
			result.Values = nil
		}

		switch opts.OutputFormat {
		case JsonOutputFormat:
			b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
			if err != nil {
				return nil, fmt.Errorf("marshal result to json: %w", err)
			}

			resultMessage = string(b)
		case YamlOutputFormat:
			b, err := yaml.MarshalContext(ctx, result)
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

		if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, string(opts.OutputFormat), colorLevel); err != nil {
			return nil, fmt.Errorf("write result to output: %w", err)
		}
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

const ReleaseGetResultApiVersionV1 = "v1"

type ReleaseGetResultV1 struct {
	ApiVersion string                   `json:"apiVersion"`
	Release    *ReleaseGetResultRelease `json:"release"`
	Chart      *ReleaseGetResultChart   `json:"chart"`
	Notes      string                   `json:"notes"`
	Values     map[string]interface{}   `json:"values"`
	Hooks      []map[string]interface{} `json:"hooks"`
	Resources  []map[string]interface{} `json:"resources"`
}

type ReleaseGetResultRelease struct {
	Name        string                      `json:"name"`
	Namespace   string                      `json:"namespace"`
	Revision    int                         `json:"revision"`
	Status      helmrelease.Status          `json:"status"`
	DeployedAt  *ReleaseGetResultDeployedAt `json:"deployedAt"`
	Annotations map[string]string           `json:"annotations"`
}

type ReleaseGetResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseGetResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}
