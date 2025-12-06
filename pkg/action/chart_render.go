package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultChartRenderLogLevel = log.ErrorLevel
)

type ChartRenderOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions

	// Chart specifies the chart to render. Can be a local directory path, chart archive,
	// OCI registry URL (oci://registry/chart), or chart repository reference (repo/chart).
	// Defaults to current directory if not specified.
	Chart string
	// ChartAppVersion overrides the appVersion field in Chart.yaml.
	// Used to set application version metadata without modifying the chart file.
	ChartAppVersion string
	// ChartDirPath is deprecated (TODO v2: remove). Use Chart instead.
	ChartDirPath string // TODO(v2): get rid
	// ChartProvenanceKeyring is the path to a keyring file containing public keys
	// used to verify chart provenance signatures. Used with signed charts for security.
	ChartProvenanceKeyring string
	// ChartProvenanceStrategy defines how to verify chart provenance.
	// Defaults to DefaultChartProvenanceStrategy if not set.
	ChartProvenanceStrategy string
	// ChartRepoSkipUpdate, when true, skips updating the chart repository cache before fetching the chart.
	// Useful for offline operations or when repository is known to be up-to-date.
	ChartRepoSkipUpdate bool
	// ChartVersion specifies the version of the chart to render (e.g., "1.2.3").
	// If not specified, the latest version is used.
	ChartVersion string
	// DefaultChartAPIVersion sets the default Chart API version when Chart.yaml doesn't specify one.
	DefaultChartAPIVersion string
	// DefaultChartName sets the default chart name when Chart.yaml doesn't specify one.
	DefaultChartName string
	// DefaultChartVersion sets the default chart version when Chart.yaml doesn't specify one.
	DefaultChartVersion string
	// ExtraAPIVersions is a list of additional Kubernetes API versions to include when rendering.
	// Used by Capabilities.APIVersions in templates to check for API availability.
	ExtraAPIVersions []string
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources.
	// These are added during chart rendering.
	ExtraAnnotations map[string]string
	// ExtraLabels are additional Kubernetes labels to add to all chart resources.
	// These are added during chart rendering.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime.
	// TODO(v2): remove or implement custom logic for this field.
	ExtraRuntimeAnnotations map[string]string // TODO(v2): get rid?? or do custom logic
	// ForceAdoption is currently unused in chart rendering.
	// TODO(v2): remove this useless field.
	ForceAdoption bool // TODO(v2): get rid, useless
	// LegacyChartType specifies the chart type for legacy compatibility.
	// Used internally for backward compatibility with werf integration.
	LegacyChartType helmopts.ChartType
	// LegacyExtraValues provides additional values programmatically.
	// Used internally for backward compatibility with werf integration.
	LegacyExtraValues map[string]interface{}
	// LegacyLogRegistryStreamOut is the output writer for Helm registry client logs.
	// Defaults to io.Discard if not set. Used for debugging registry operations.
	LegacyLogRegistryStreamOut io.Writer
	// LocalKubeVersion specifies the Kubernetes version to use for template rendering when not connected to a cluster.
	// Format: "major.minor.patch" (e.g., "1.28.0"). Defaults to DefaultLocalKubeVersion if not set.
	LocalKubeVersion string
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// OutputFilePath, if specified, writes the rendered manifests to this file instead of stdout.
	OutputFilePath string
	// OutputNoPrint, when true, suppresses printing the rendered manifests to stdout.
	// Useful when only the result data structure is needed.
	OutputNoPrint bool
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultRegistryCredentialsPath (~/.docker/config.json) if not set.
	// Used for authenticating to OCI registries when pulling charts.
	RegistryCredentialsPath string
	// ReleaseName is the name of the release to use in templates.
	// Available as .Release.Name in chart templates.
	ReleaseName string
	// ReleaseNamespace is the namespace where the release would be installed.
	// Available as .Release.Namespace in chart templates.
	ReleaseNamespace string
	// ReleaseStorageDriver specifies how release metadata would be stored (affects template rendering).
	// Valid values: "secret" (default), "configmap", "sql", "memory".
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// Remote, when true, connects to a real Kubernetes cluster to fetch capabilities and validate API versions.
	// When false, uses local/stub Kubernetes version for rendering.
	Remote bool
	// ShowOnlyFiles, if specified, filters output to only show resources from these file paths.
	// Paths are relative to the chart directory (e.g., "templates/deployment.yaml").
	ShowOnlyFiles []string
	// ShowStandaloneCRDs, when true, includes CustomResourceDefinitions from the "crds/" directory in the output.
	// By default, CRDs are hidden from rendered output.
	ShowStandaloneCRDs bool
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
	// TemplatesAllowDNS, when true, enables DNS lookups in chart templates using template functions.
	// WARNING: This can make template rendering non-deterministic and slower.
	TemplatesAllowDNS bool
}

// Render the Helm chart.
func ChartRender(ctx context.Context, opts ChartRenderOptions) (*ChartRenderResultV2, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyChartRenderOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build chart render options: %w", err)
	}

	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	if !opts.Remote {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverMemory
	}

	var clientFactory *kube.ClientFactory

	if opts.Remote {
		if len(opts.KubeConfigPaths) > 0 {
			var splitPaths []string
			for _, path := range opts.KubeConfigPaths {
				splitPaths = append(splitPaths, filepath.SplitList(path)...)
			}

			opts.KubeConfigPaths = lo.Compact(splitPaths)
		}

		kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
			KubeConnectionOptions: opts.KubeConnectionOptions,
			KubeContextNamespace:  opts.ReleaseNamespace, // TODO: unset it everywhere
		})
		if err != nil {
			return nil, fmt.Errorf("construct kube config: %w", err)
		}

		clientFactory, err = kube.NewClientFactory(ctx, kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("construct kube client factory: %w", err)
		}
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
		registry.ClientOptWriter(opts.LegacyLogRegistryStreamOut),
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
	}

	if opts.ChartRepoInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("construct registry client: %w", err)
	}

	releaseStorageOptions := release.ReleaseStorageOptions{
		SQLConnection: opts.ReleaseStorageSQLConnection,
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, opts.ReleaseNamespace, opts.ReleaseStorageDriver, clientFactory, releaseStorageOptions)
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
	}

	helmOptions := helmopts.HelmOptions{
		ChartLoadOpts: helmopts.ChartLoadOptions{
			ChartAppVersion:            opts.ChartAppVersion,
			ChartType:                  opts.LegacyChartType,
			DefaultChartAPIVersion:     opts.DefaultChartAPIVersion,
			DefaultChartName:           opts.DefaultChartName,
			DefaultChartVersion:        opts.DefaultChartVersion,
			DefaultSecretValuesDisable: opts.DefaultSecretValuesDisable,
			DefaultValuesDisable:       opts.DefaultValuesDisable,
			ExtraValues:                opts.LegacyExtraValues,
			SecretKeyIgnore:            opts.SecretKeyIgnore,
			SecretValuesFiles:          opts.SecretValuesFiles,
			SecretWorkDir:              opts.SecretWorkDir,
		},
	}

	log.Default.Debug(ctx, "Build release history")

	history, err := release.BuildHistory(opts.ReleaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return nil, fmt.Errorf("build release history: %w", err)
	}

	releases := history.Releases()
	deployedReleases := history.FindAllDeployed()
	prevRelease := lo.LastOrEmpty(releases)
	prevDeployedRelease := lo.LastOrEmpty(deployedReleases)

	var newRevision int
	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
	} else {
		newRevision = 1
	}

	var deployType common.DeployType
	if prevDeployedRelease != nil {
		deployType = common.DeployTypeUpgrade
	} else if prevRelease != nil {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
	}

	chartTreeOptions := chart.RenderChartOptions{
		ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
		ValuesOptions:              opts.ValuesOptions,
		ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
		ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
		ChartRepoNoUpdate:          opts.ChartRepoSkipUpdate,
		ChartVersion:               opts.ChartVersion,
		ExtraAPIVersions:           opts.ExtraAPIVersions,
		HelmOptions:                helmOptions,
		LocalKubeVersion:           opts.LocalKubeVersion,
		Remote:                     opts.Remote,
		TemplatesAllowDNS:          opts.TemplatesAllowDNS,
	}

	log.Default.Debug(ctx, "Render chart")

	renderChartResult, err := chart.RenderChart(ctx, opts.Chart, opts.ReleaseName, opts.ReleaseNamespace, newRevision, deployType, helmRegistryClient, clientFactory, chartTreeOptions)
	if err != nil {
		return nil, fmt.Errorf("render chart: %w", err)
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, opts.ReleaseNamespace, renderChartResult.ResourceSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return nil, fmt.Errorf("build transformed resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build releasable resource specs")

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, opts.ReleaseNamespace, transformedResSpecs, []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
		spec.NewSecretStringDataPatcher(),
	})
	if err != nil {
		return nil, fmt.Errorf("build releasable resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(opts.ReleaseName, opts.ReleaseNamespace, newRevision, deployType, releasableResSpecs, renderChartResult.Chart, renderChartResult.ReleaseConfig, release.ReleaseOptions{})
	if err != nil {
		return nil, fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	resSpecs, err := release.ReleaseToResourceSpecs(newRelease, opts.ReleaseNamespace, true)
	if err != nil {
		return nil, fmt.Errorf("convert new release to resource specs: %w", err)
	}

	var showFiles []string

	for _, file := range opts.ShowOnlyFiles {
		absFile, err := filepath.Abs(file)
		if err != nil {
			return nil, fmt.Errorf("get absolute path for %q: %w", file, err)
		}

		if strings.HasPrefix(absFile, opts.Chart) {
			f, err := filepath.Rel(opts.Chart, absFile)
			if err != nil {
				return nil, fmt.Errorf("get relative path for %q: %w", absFile, err)
			}

			if !strings.HasPrefix(f, renderChartResult.Chart.Name()) {
				f = filepath.Join(renderChartResult.Chart.Name(), f)
			}

			showFiles = append(showFiles, f)
		} else {
			if !strings.HasPrefix(file, renderChartResult.Chart.Name()) {
				file = filepath.Join(renderChartResult.Chart.Name(), file)
			}

			showFiles = append(showFiles, file)
		}
	}

	var (
		renderOutStream  io.Writer
		renderColorLevel color.Level
	)

	if opts.OutputFilePath != "" {
		file, err := os.Create(opts.OutputFilePath)
		if err != nil {
			return nil, fmt.Errorf("create chart render output file %q: %w", opts.OutputFilePath, err)
		}
		defer file.Close()

		renderOutStream = file
		renderColorLevel = color.LevelNo
	} else {
		renderOutStream = os.Stdout

		if color.Enable {
			renderColorLevel = color.TermColorLevel()
		}
	}

	result := &ChartRenderResultV2{
		APIVersion: "v2",
		Resources:  resSpecs,
	}

	sort.SliceStable(result.Resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(result.Resources[i], result.Resources[j])
	})

	for _, res := range result.Resources {
		if len(showFiles) > 0 && !lo.Contains(showFiles, res.FilePath) {
			continue
		}

		if !opts.ShowStandaloneCRDs && res.StoreAs == common.StoreAsNone &&
			spec.IsCRD(res.GroupVersionKind.GroupKind()) {
			continue
		}

		if err := renderResource(res.Unstruct, res.FilePath, renderOutStream, renderColorLevel); err != nil {
			return nil, fmt.Errorf("render resource %q: %w", res.IDHuman(), err)
		}
	}

	return result, nil
}

func applyChartRenderOptionsDefaults(opts ChartRenderOptions, currentDir, homeDir string) (ChartRenderOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ChartRenderOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.ChartRepoConnectionOptions.ApplyDefaults()
	opts.ValuesOptions.ApplyDefaults()
	opts.SecretValuesOptions.ApplyDefaults(currentDir)

	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	if opts.ReleaseName == "" {
		opts.ReleaseName = common.StubReleaseName
	}

	if opts.ReleaseNamespace == "" {
		opts.ReleaseNamespace = common.StubReleaseNamespace
	}

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	}

	if opts.LocalKubeVersion == "" {
		// TODO(v3): update default local version
		opts.LocalKubeVersion = common.DefaultLocalKubeVersion
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = common.DefaultRegistryCredentialsPath
	}

	if opts.ChartProvenanceStrategy == "" {
		opts.ChartProvenanceStrategy = common.DefaultChartProvenanceStrategy
	}

	return opts, nil
}

func renderResource(unstruct *unstructured.Unstructured, path string, outStream io.Writer, colorLevel color.Level) error {
	resourceJSONBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, unstruct)
	if err != nil {
		return fmt.Errorf("encode to JSON: %w", err)
	}

	resourceYamlBytes, err := yaml.JSONToYAML(resourceJSONBytes)
	if err != nil {
		return fmt.Errorf("marshal JSON to YAML: %w", err)
	}

	prefix := fmt.Sprintf("---\n# Source: %s\n", path)
	manifest := prefix + string(resourceYamlBytes)

	if err := writeWithSyntaxHighlight(outStream, manifest, "yaml", colorLevel); err != nil {
		return fmt.Errorf("write resource to output: %w", err)
	}

	return nil
}

type ChartRenderResultV2 struct {
	APIVersion string               `json:"apiVersion,omitempty"`
	Resources  []*spec.ResourceSpec `json:"resources,omitempty"`
}
