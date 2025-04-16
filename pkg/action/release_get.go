package action

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gookit/color"
	"github.com/samber/lo"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart/loader"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/log"
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
	LogColorMode         string
	NetworkParallelism   int
	OutputFormat         string
	OutputNoPrint        bool
	ReleaseStorageDriver string
	Revision             int
	TempDirPath          string
}

func ReleaseGet(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseGetOptions) (*ReleaseGetResultV1, error) {
	actionLock.Lock()
	defer actionLock.Unlock()

	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyReleaseGetOptionsDefaults(opts, currentUser)
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

	helmSettings := helm_v3.Settings
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))

	helmActionConfig := &action.Configuration{}
	if err := helmActionConfig.Init(
		clientFactory.LegacyClientGetter(),
		releaseNamespace,
		string(opts.ReleaseStorageDriver),
		func(format string, a ...interface{}) {
			log.Default.Debug(ctx, format, a...)
		},
	); err != nil {
		return nil, fmt.Errorf("helm action config init: %w", err)
	}

	helmReleaseStorage := helmActionConfig.Releases

	secrets.DisableSecrets = true
	loader.NoChartLockWarning = ""

	history, err := release.NewHistory(
		releaseName,
		releaseNamespace,
		helmReleaseStorage,
		release.HistoryOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("construct release history: %w", err)
	}

	var (
		release      *release.Release
		releaseFound bool
	)
	if opts.Revision == 0 {
		release, releaseFound, err = history.LastRelease()
		if err != nil {
			return nil, fmt.Errorf("get last release: %w", err)
		}
	} else {
		release, releaseFound, err = history.Release(opts.Revision)
		if err != nil {
			return nil, fmt.Errorf("get release revision %d: %w", opts.Revision, err)
		}
	}

	if !releaseFound {
		if opts.Revision == 0 {
			return nil, fmt.Errorf("release %q (namespace %q) not found", releaseName, releaseNamespace)
		} else {
			return nil, fmt.Errorf("revision %d of release %q (namespace %q) not found", opts.Revision, releaseName, releaseNamespace)
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
		Notes: release.Notes(),
	}

	for _, hook := range release.HookResources() {
		result.Hooks = append(result.Hooks, hook.Unstructured().Object)
	}

	for _, resource := range release.GeneralResources() {
		result.Resources = append(result.Resources, resource.Unstructured().Object)
	}

	if !opts.OutputNoPrint {
		var resultMessage string

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

		var colorLevel color.Level
		if opts.LogColorMode != LogColorModeOff {
			colorLevel = color.DetectColorLevel()
		}

		if err := writeWithSyntaxHighlight(os.Stdout, resultMessage, string(opts.OutputFormat), colorLevel); err != nil {
			return nil, fmt.Errorf("write result to output: %w", err)
		}
	}

	return result, nil
}

func applyReleaseGetOptionsDefaults(opts ReleaseGetOptions, currentUser *user.User) (ReleaseGetOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseGetOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

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
