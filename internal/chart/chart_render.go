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
	"unicode"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/cli"
	"github.com/werf/3p-helm/pkg/cli/values"
	"github.com/werf/3p-helm/pkg/downloader"
	helmengine "github.com/werf/3p-helm/pkg/engine"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

// TODO(ilya-lesikov): pass missing options from top-level
type RenderChartOptions struct {
	AllowDNSRequests       bool
	ChartRepoInsecure      bool
	ChartRepoSkipTLSVerify bool
	ChartRepoSkipUpdate    bool
	ChartVersion           string
	FileValues             []string
	HelmOptions            helmopts.HelmOptions
	KubeCAPath             string
	KubeVersion            *chartutil.KubeVersion
	NoStandaloneCRDs       bool
	RegistryClient         *registry.Client
	Remote                 bool
	SetValues              []string
	StringSetValues        []string
	SubNotes               bool
	ValuesFiles            []string
}

type RenderChartResult struct {
	Chart         *chart.Chart
	Notes         string
	ReleaseConfig map[string]interface{}
	ResourceSpecs []*spec.ResourceSpec
	Values        map[string]interface{}
}

func RenderChart(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, clientFactory kube.ClientFactorier, opts RenderChartOptions) (*RenderChartResult, error) {
	chartPath, err := downloadChart(ctx, chartPath, opts)
	if err != nil {
		return nil, fmt.Errorf("download chart %q: %w", chartPath, err)
	}

	depDownloader := &downloader.Manager{
		Out:               os.Stdout,
		ChartPath:         chartPath,
		SkipUpdate:        opts.ChartRepoSkipUpdate,
		AllowMissingRepos: true,
		Getters:           getter.Providers{getter.HttpProvider, getter.OCIProvider},
		RegistryClient:    opts.RegistryClient,
		// TODO(v3): don't read HELM_REPOSITORY_CONFIG anymore
		RepositoryConfig: cli.EnvOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(v3): don't read HELM_REPOSITORY_CACHE anymore
		RepositoryCache: cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
		Debug:           log.Default.AcceptLevel(ctx, log.DebugLevel),
	}

	opts.HelmOptions.ChartLoadOpts.DepDownloader = depDownloader

	overrideValuesOpts := &values.Options{
		StringValues: opts.StringSetValues,
		Values:       opts.SetValues,
		FileValues:   opts.FileValues,
		ValueFiles:   opts.ValuesFiles,
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
		var e *downloader.ErrRepoNotFound
		if errors.As(err, &e) {
			return nil, fmt.Errorf("%w. Please add the missing repos via 'helm repo add'", e)
		}

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

	// TODO(ilya-lesikov): pass custom local api versions
	caps, err := buildChartCapabilities(ctx, clientFactory, buildChartCapabilitiesOptions{
		Remote:      opts.Remote,
		KubeVersion: opts.KubeVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("build capabilities for chart %q: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, caps, "Capabilities:")

	if chart.Metadata.KubeVersion != "" && !chartutil.IsCompatibleRange(chart.Metadata.KubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", chart.Metadata.KubeVersion, caps.KubeVersion.String())
	}

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
	}, caps)
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

	engine.EnableDNS = opts.AllowDNSRequests

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

	notes := buildChartNotes(chart.Name(), renderedTemplates, opts.SubNotes)

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
	APIVersions *chartutil.VersionSet
	KubeVersion *chartutil.KubeVersion
	Remote      bool
}

func buildChartCapabilities(ctx context.Context, clientFactory kube.ClientFactorier, opts buildChartCapabilitiesOptions) (*chartutil.Capabilities, error) {
	capabilities := &chartutil.Capabilities{
		HelmVersion: chartutil.DefaultCapabilities.HelmVersion,
	}

	if opts.Remote {
		clientFactory.Discovery().Invalidate()

		if opts.KubeVersion != nil {
			capabilities.KubeVersion = *opts.KubeVersion
		} else {
			kubeVersion, err := clientFactory.Discovery().ServerVersion()
			if err != nil {
				return nil, fmt.Errorf("get kubernetes server version: %w", err)
			}

			capabilities.KubeVersion = chartutil.KubeVersion{
				Version: kubeVersion.GitVersion,
				Major:   kubeVersion.Major,
				Minor:   kubeVersion.Minor,
			}
		}

		if opts.APIVersions != nil {
			capabilities.APIVersions = *opts.APIVersions
		} else {
			apiVersions, err := action.GetVersionSet(clientFactory.Discovery())
			if err != nil {
				if discovery.IsGroupDiscoveryFailedError(err) {
					log.Default.Warn(ctx, "Discovery failed: %s", err.Error())
				} else {
					return nil, fmt.Errorf("get version set: %w", err)
				}
			}

			capabilities.APIVersions = apiVersions
		}
	} else {
		if opts.KubeVersion != nil {
			capabilities.KubeVersion = *opts.KubeVersion
		} else {
			capabilities.KubeVersion = chartutil.DefaultCapabilities.KubeVersion
		}

		if opts.APIVersions != nil {
			capabilities.APIVersions = *opts.APIVersions
		} else {
			capabilities.APIVersions = chartutil.DefaultCapabilities.APIVersions
		}
	}

	return capabilities, nil
}
