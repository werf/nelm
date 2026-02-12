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
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseGetOutputFormat = common.OutputFormatYAML
	DefaultReleaseGetLogLevel     = log.ErrorLevel
)

type ReleaseGetOptions struct {
	common.KubeConnectionOptions

	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// OutputFormat specifies the output format for the release information.
	// Valid values: "yaml" (default), "json", "table".
	// Defaults to DefaultReleaseGetOutputFormat (yaml) if not specified.
	OutputFormat string
	// OutputNoPrint, when true, suppresses printing the output and only returns the result data structure.
	// Useful when calling this programmatically.
	OutputNoPrint bool
	// PrintValues, when true, includes the computed values used to render the release in the output.
	// These are the merged values from all sources (values.yaml, --set flags, etc.).
	PrintValues bool
	// ReleaseStorageDriver specifies how release metadata is stored in Kubernetes.
	// Valid values: "secret" (default), "configmap", "sql".
	// Defaults to "secret" if not specified or set to "default".
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// Revision specifies which release revision to retrieve.
	// If 0, retrieves the latest deployed revision.
	Revision int
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
}

// Retrieves detailed information about the Helm release from the cluster.
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
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace, // TODO: unset it everywhere
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

	resSpecs, err := release.ReleaseToResourceSpecs(rel, releaseNamespace, false)
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
	case common.OutputFormatJSON:
		b, err := json.MarshalIndent(result, "", strings.Repeat(" ", 2))
		if err != nil {
			return nil, fmt.Errorf("marshal result to json: %w", err)
		}

		resultMessage = string(b)
	case common.OutputFormatYAML:
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

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
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
	// TODO(major): Join Hooks and Resources together as ResourceSpecs?
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

// TODO(major): get rid
type ReleaseGetResultDeployedAt struct {
	Human string `json:"human"`
	Unix  int    `json:"unix"`
}

type ReleaseGetResultChart struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
}
