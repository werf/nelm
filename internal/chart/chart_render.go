package chart

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/samber/lo"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/cli"
	"github.com/werf/3p-helm/pkg/cli/values"
	helmdownloader "github.com/werf/3p-helm/pkg/downloader"
	helmengine "github.com/werf/3p-helm/pkg/engine"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/3p-helm/pkg/strvals"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

// TODO(ilya-lesikov): pass missing options from top-level
type RenderChartOptions struct {
	ChartProvenanceKeyring     string
	ChartProvenanceStrategy    string
	ChartRepoBasicAuthPassword string
	ChartRepoBasicAuthUsername string
	ChartRepoCAPath            string
	ChartRepoCertPath          string
	ChartRepoInsecure          bool
	ChartRepoKeyPath           string
	ChartRepoNoTLSVerify       bool
	ChartRepoNoUpdate          bool
	ChartRepoPassCreds         bool
	ChartRepoRequestTimeout    time.Duration
	ChartRepoURL               string
	ChartVersion               string
	ExtraAPIVersions           []string
	HelmOptions                helmopts.HelmOptions
	LocalKubeVersion           string
	NoStandaloneCRDs           bool
	Remote                     bool
	RuntimeSetJSON             []string
	SubchartNotes              bool
	TemplatesAllowDNS          bool
	ValuesFiles                []string
	ValuesSet                  []string
	ValuesSetFile              []string
	ValuesSetJSON              []string
	ValuesSetLiteral           []string
	ValuesSetString            []string
}

type RenderChartResult struct {
	Chart         *chart.Chart
	Notes         string
	ReleaseConfig map[string]interface{}
	ResourceSpecs []*spec.ResourceSpec
	Values        map[string]interface{}
}

func RenderChart(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, registryClient *registry.Client, clientFactory kube.ClientFactorier, opts RenderChartOptions) (*RenderChartResult, error) {
	chartPath, err := downloadChart(ctx, chartPath, registryClient, opts)
	if err != nil {
		return nil, fmt.Errorf("download chart %q: %w", chartPath, err)
	}

	depDownloader := &helmdownloader.Manager{
		Out:            os.Stdout,
		ChartPath:      chartPath,
		Verify:         helmdownloader.VerificationStrategyString(opts.ChartProvenanceStrategy).ToVerificationStrategy(),
		Debug:          log.Default.AcceptLevel(ctx, log.DebugLevel),
		Keyring:        opts.ChartProvenanceKeyring,
		SkipUpdate:     opts.ChartRepoNoUpdate,
		Getters:        getter.Providers{getter.HttpProvider, getter.OCIProvider},
		RegistryClient: registryClient,
		// TODO(v2): don't read HELM_REPOSITORY_CONFIG anymore
		RepositoryConfig: cli.EnvOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(v2): don't read HELM_REPOSITORY_CACHE anymore
		RepositoryCache:   cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
		AllowMissingRepos: true,
	}

	opts.HelmOptions.ChartLoadOpts.DepDownloader = depDownloader

	overrideValuesOpts := &values.Options{
		ValueFiles:    opts.ValuesFiles,
		StringValues:  opts.ValuesSetString,
		Values:        opts.ValuesSet,
		FileValues:    opts.ValuesSetFile,
		JSONValues:    opts.ValuesSetJSON,
		LiteralValues: opts.ValuesSetLiteral,
	}

	log.Default.TraceStruct(ctx, overrideValuesOpts, "Override values options:")
	log.Default.Debug(ctx, "Merging override values for chart at %q", chartPath)

	overrideValues, err := overrideValuesOpts.MergeValues(getter.Providers{getter.HttpProvider, getter.OCIProvider}, opts.HelmOptions)
	if err != nil {
		return nil, fmt.Errorf("merge override values for chart at %q: %w", chartPath, err)
	}

	log.Default.TraceStruct(ctx, overrideValues, "Merged override values:")
	log.Default.Debug(ctx, "Loading chart at %q", chartPath)

	chart, err := loader.Load(chartPath, opts.HelmOptions)
	if err != nil {
		return nil, fmt.Errorf("load chart at %q: %w", chartPath, err)
	}

	if err := validateChart(ctx, chart); err != nil {
		return nil, fmt.Errorf("validate chart at %q: %w", chartPath, err)
	}

	log.Default.TraceStruct(ctx, chart, "Chart:")

	if err := chartutil.ProcessDependenciesWithMerge(chart, &overrideValues); err != nil {
		return nil, fmt.Errorf("process chart %q dependencies: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, chart, "Chart after processing dependencies:")
	log.Default.TraceStruct(ctx, overrideValues, "Merged override values after processing dependencies:")

	caps, err := buildChartCapabilities(ctx, clientFactory, buildChartCapabilitiesOptions{
		ExtraAPIVersions: opts.ExtraAPIVersions,
		LocalKubeVersion: opts.LocalKubeVersion,
		Remote:           opts.Remote,
	})
	if err != nil {
		return nil, fmt.Errorf("build capabilities for chart %q: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, caps, "Capabilities:")

	if chart.Metadata.KubeVersion != "" && !chartutil.IsCompatibleRange(chart.Metadata.KubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", chart.Metadata.KubeVersion, caps.KubeVersion.String())
	}

	runtime, err := buildRuntime(opts.RuntimeSetJSON)
	if err != nil {
		return nil, fmt.Errorf("build runtime: %w", err)
	}

	log.Default.TraceStruct(ctx, runtime, "Runtime:")

	var isUpgrade bool
	switch deployType {
	case common.DeployTypeUpgrade, common.DeployTypeRollback:
		isUpgrade = true
	case common.DeployTypeInitial, common.DeployTypeInstall:
		isUpgrade = false
	default:
		panic("unexpected deployType")
	}

	log.Default.Debug(ctx, "Rendering values for chart at %q", chartPath)

	renderedValues, err := chartutil.ToRenderValues(chart, overrideValues, chartutil.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps, runtime)
	if err != nil {
		return nil, fmt.Errorf("build rendered values for chart %q: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, renderedValues.AsMap(), "Rendered values:")

	var engine *helmengine.Engine
	if opts.Remote && clientFactory.KubeClient() != nil {
		engine = lo.ToPtr(helmengine.New(clientFactory.KubeConfig().RestConfig))
	} else {
		engine = lo.ToPtr(helmengine.Engine{})
	}

	engine.EnableDNS = opts.TemplatesAllowDNS

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)

	var resources []*spec.ResourceSpec

	if !opts.NoStandaloneCRDs {
		for _, crd := range chart.CRDObjects() {
			for _, manifest := range releaseutil.SplitManifests(string(crd.File.Data)) {
				if res, err := spec.NewResourceSpecFromManifest(manifest, releaseNamespace, spec.ResourceSpecOptions{
					StoreAs:  common.StoreAsNone,
					FilePath: crd.Filename,
				}); err != nil {
					return nil, fmt.Errorf("construct standalone CRD for chart at %q: %w", chartPath, err)
				} else {
					resources = append(resources, res)
				}
			}
		}
	}

	renderedTemplates, err := engine.Render(chart, renderedValues, opts.HelmOptions)
	if err != nil {
		return nil, fmt.Errorf("render resources for chart %q: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, renderedTemplates, "Rendered contents of templates/:")

	if r, err := renderedTemplatesToResourceSpecs(renderedTemplates, releaseNamespace, opts); err != nil {
		return nil, fmt.Errorf("convert rendered templates to installable resources for chart at %q: %w", chartPath, err)
	} else {
		resources = append(resources, r...)
	}

	notes := buildChartNotes(chart.Name(), renderedTemplates, opts.SubchartNotes)

	log.Default.TraceStruct(ctx, notes, "Rendered notes:")

	sort.SliceStable(resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(resources[i], resources[j])
	})

	return &RenderChartResult{
		Chart:         chart,
		Notes:         notes,
		ReleaseConfig: overrideValues,
		ResourceSpecs: resources,
		Values:        renderedValues.AsMap(),
	}, nil
}

func validateChart(ctx context.Context, chart *chart.Chart) error {
	if chart == nil {
		return fmt.Errorf("load chart: %w", action.ErrMissingChart())
	}

	if chart.Metadata.Type != "" && chart.Metadata.Type != "application" {
		return fmt.Errorf("chart %q of type %q can't be deployed", chart.Name(), chart.Metadata.Type)
	}

	if chart.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(chart, chart.Metadata.Dependencies); err != nil {
			return fmt.Errorf("check chart dependencies for chart %q: %w", chart.Name(), err)
		}
	}

	if chart.Metadata.Deprecated {
		log.Default.Warn(ctx, `Chart "%s:%s" is deprecated`, chart.Name(), chart.Metadata.Version)
	}

	return nil
}

func renderedTemplatesToResourceSpecs(renderedTemplates map[string]string, releaseNamespace string, opts RenderChartOptions) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec
	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, action.NotesFileSuffix) ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		manifests := releaseutil.SplitManifests(fileContent)

		for _, manifest := range manifests {
			var head releaseutil.SimpleHead
			if err := yaml.Unmarshal([]byte(manifest), &head); err != nil {
				return nil, fmt.Errorf("parse YAML for %q: %w", filePath, err)
			}

			if res, err := spec.NewResourceSpecFromManifest(manifest, releaseNamespace, spec.ResourceSpecOptions{
				FilePath: filePath,
			}); err != nil {
				return nil, fmt.Errorf("construct resource spec for %q: %w", filePath, err)
			} else {
				resources = append(resources, res)
			}
		}
	}

	return resources, nil
}

func isLocalChart(path string) bool {
	return filepath.IsAbs(path) || filepath.HasPrefix(path, "..") || filepath.HasPrefix(path, ".")
}

func buildChartNotes(chartName string, renderedTemplates map[string]string, renderSubchartNotes bool) string {
	var resultBuf bytes.Buffer

	for filePath, fileContent := range renderedTemplates {
		if !strings.HasSuffix(filePath, action.NotesFileSuffix) {
			continue
		}

		fileContent = strings.TrimRightFunc(fileContent, unicode.IsSpace)
		if fileContent == "" {
			continue
		}

		isTopLevelNotes := filePath == path.Join(chartName, "templates", action.NotesFileSuffix)

		if !isTopLevelNotes && !renderSubchartNotes {
			continue
		}

		if resultBuf.Len() > 0 {
			resultBuf.WriteString("\n")
		}

		resultBuf.WriteString(fileContent)
	}

	return resultBuf.String()
}

type buildChartCapabilitiesOptions struct {
	ExtraAPIVersions []string
	LocalKubeVersion string
	Remote           bool
}

func buildChartCapabilities(ctx context.Context, clientFactory kube.ClientFactorier, opts buildChartCapabilitiesOptions) (*chartutil.Capabilities, error) {
	capabilities := &chartutil.Capabilities{
		HelmVersion: chartutil.DefaultCapabilities.HelmVersion,
	}

	if opts.Remote {
		clientFactory.Discovery().Invalidate()

		kubeVersion, err := clientFactory.Discovery().ServerVersion()
		if err != nil {
			return nil, fmt.Errorf("get kubernetes server version: %w", err)
		}

		capabilities.KubeVersion = chartutil.KubeVersion{
			Version: kubeVersion.GitVersion,
			Major:   kubeVersion.Major,
			Minor:   kubeVersion.Minor,
		}

		apiVersions, err := action.GetVersionSet(clientFactory.Discovery())
		if err != nil {
			if discovery.IsGroupDiscoveryFailedError(err) {
				log.Default.Warn(ctx, "Discovery failed: %s", err.Error())
			} else {
				return nil, fmt.Errorf("get version set: %w", err)
			}
		}

		capabilities.APIVersions = apiVersions
	} else {
		if opts.LocalKubeVersion != "" {
			kubeVersion, err := chartutil.ParseKubeVersion(opts.LocalKubeVersion)
			if err != nil {
				return nil, fmt.Errorf("parse kube version %q: %w", opts.LocalKubeVersion, err)
			}

			capabilities.KubeVersion = *kubeVersion
		} else {
			capabilities.KubeVersion = chartutil.DefaultCapabilities.KubeVersion
		}

		capabilities.APIVersions = chartutil.DefaultCapabilities.APIVersions
	}

	if opts.ExtraAPIVersions != nil {
		capabilities.APIVersions = append(capabilities.APIVersions, chartutil.VersionSet(opts.ExtraAPIVersions)...)
	}

	return capabilities, nil
}

func buildRuntime(jsonSets []string) (map[string]interface{}, error) {
	runtime := map[string]interface{}{}

	for _, jsonSet := range jsonSets {
		if err := strvals.ParseJSON(jsonSet, runtime); err != nil {
			return nil, fmt.Errorf("parse runtime JSON set %q: %w", jsonSet, err)
		}
	}

	return runtime, nil
}
