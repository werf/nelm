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

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlshistor"
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
	LogLevel             string
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

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, log.Level(opts.LogLevel))
	} else {
		log.Default.SetLevel(ctx, log.Level(DefaultReleaseGetLogLevel))
	}

	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyReleaseGetOptionsDefaults(opts, currentUser)
	if err != nil {
		return nil, fmt.Errorf("build release get options: %w", err)
	}

	var kubeConfigPath string
	if len(opts.KubeConfigPaths) > 0 {
		kubeConfigPath = opts.KubeConfigPaths[0]
	}

	kubeConfigGetter, err := kube.NewKubeConfigGetter(
		kube.KubeConfigGetterOptions{
			KubeConfigOptions: kube.KubeConfigOptions{
				Context:             opts.KubeContext,
				ConfigPath:          kubeConfigPath,
				ConfigDataBase64:    opts.KubeConfigBase64,
				ConfigPathMergeList: opts.KubeConfigPaths,
			},
			Namespace:     releaseNamespace,
			BearerToken:   opts.KubeToken,
			APIServer:     opts.KubeAPIServerName,
			CAFile:        opts.KubeCAPath,
			TLSServerName: opts.KubeTLSServerName,
			SkipTLSVerify: opts.KubeSkipTLSVerify,
			QPSLimit:      opts.KubeQPSLimit,
			BurstLimit:    opts.KubeBurstLimit,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("construct kube config getter: %w", err)
	}

	helmSettings := helm_v3.Settings
	*helmSettings.GetConfigP() = kubeConfigGetter
	*helmSettings.GetNamespaceP() = releaseNamespace
	releaseNamespace = helmSettings.Namespace()
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
	}

	helmActionConfig := &action.Configuration{}
	if err := helmActionConfig.Init(
		helmSettings.RESTClientGetter(),
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

	history, err := rlshistor.NewHistory(
		releaseName,
		releaseNamespace,
		helmReleaseStorage,
		rlshistor.HistoryOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("construct release history: %w", err)
	}

	var (
		release      *rls.Release
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

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
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
	Status      release.Status              `json:"status"`
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
