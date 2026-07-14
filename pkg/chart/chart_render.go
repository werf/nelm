package chart

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/pkg/helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/helm/pkg/cli"
	"github.com/werf/nelm/pkg/helm/pkg/cli/values"
	helmdownloader "github.com/werf/nelm/pkg/helm/pkg/downloader"
	helmengine "github.com/werf/nelm/pkg/helm/pkg/engine"
	"github.com/werf/nelm/pkg/helm/pkg/getter"
	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/helm/pkg/registry"
	"github.com/werf/nelm/pkg/helm/pkg/releaseutil"
	"github.com/werf/nelm/pkg/helm/pkg/strvals"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/lookup"
	"github.com/werf/nelm/pkg/resource/spec"
	"github.com/werf/nelm/pkg/ts"
)

type RenderChartOptions struct {
	common.ChartRepoConnectionOptions
	common.ValuesOptions

	ChartProvenanceKeyring    string
	ChartProvenanceStrategy   string
	ChartRepoNoUpdate         bool
	ChartVersion              string
	DenoBinaryPath            string
	ExtraAPIVersions          []string
	HelmOptions               helmopts.HelmOptions
	IgnoreBundleJS            bool
	LocalKubeVersion          string
	LocalLookupResourcesPaths []string
	NoStandaloneCRDs          bool
	Remote                    bool
	SubchartNotes             bool
	TempDirPath               string
	TemplatesAllowDNS         bool
}

type RenderChartResult struct {
	Chart         *helmchart.Chart
	Notes         string
	ReleaseConfig map[string]interface{}
	ResourceSpecs []*spec.ResourceSpec
	Values        map[string]interface{}
}

type buildChartCapabilitiesOptions struct {
	ExtraAPIVersions []string
	LocalKubeVersion string
	Remote           bool
}

// Download chart and its dependencies, build and merge values, then render templates. Most of the
// logic is in Helm SDK, in Nelm its mostly orchestration level.
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
		// TODO(major): don't read HELM_REPOSITORY_CONFIG anymore
		RepositoryConfig: cli.EnvOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(major): don't read HELM_REPOSITORY_CACHE anymore
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

	runtime, err := buildContextFromJSONSets(opts.RuntimeSetJSON)
	if err != nil {
		return nil, fmt.Errorf("build runtime: %w", err)
	}

	log.Default.TraceStruct(ctx, runtime, "Runtime:")

	opts.HelmOptions.ChartLoadOpts.DefaultRootContext, err = buildContextFromJSONSets(opts.RootSetJSON)
	if err != nil {
		return nil, fmt.Errorf("build default root context: %w", err)
	}

	log.Default.TraceStruct(ctx, opts.HelmOptions.ChartLoadOpts.DefaultRootContext, "Default root context:")

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
	}, caps, runtime, opts.HelmOptions.ChartLoadOpts.DefaultRootContext)
	if err != nil {
		return nil, fmt.Errorf("build rendered values for chart %q: %w", chart.Name(), err)
	}

	log.Default.TraceStruct(ctx, renderedValues.AsMap(), "Rendered values:")

	var engine *helmengine.Engine
	if opts.Remote && clientFactory.KubeClient() != nil {
		engine = lo.ToPtr(helmengine.New(clientFactory.KubeConfig().RestConfig))
	} else {
		engine = &helmengine.Engine{}
		if len(opts.LocalLookupResourcesPaths) > 0 {
			localLookupResources, err := parseLocalLookupResources(opts.LocalLookupResourcesPaths)
			if err != nil {
				return nil, fmt.Errorf("parse local lookup resources: %w", err)
			}

			engine.SetClientProvider(lookup.NewLocalClientProvider(localLookupResources))
		}
	}

	engine.EnableDNS = opts.TemplatesAllowDNS

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)

	var resources []*spec.ResourceSpec

	if !opts.NoStandaloneCRDs {
		for _, crd := range chart.CRDObjects() {
			for _, manifest := range releaseutil.SplitManifestsToSlice(string(crd.File.Data)) {
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

	if featgate.FeatGateTypescript.Enabled() {
		log.Default.Debug(ctx, "Rendering TypeScript resources for chart %q and its dependencies", chart.Name())

		jsRenderedTemplates, err := ts.RenderChart(ctx, chart, renderedValues, opts.IgnoreBundleJS, chartPath, opts.TempDirPath, opts.DenoBinaryPath)
		if err != nil {
			return nil, fmt.Errorf("render TypeScript templates for chart %q: %w", chart.Name(), err)
		}

		if len(jsRenderedTemplates) > 0 {
			maps.Copy(renderedTemplates, jsRenderedTemplates)
		}
	}

	log.Default.Debug(ctx, "Rendered content:")

	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, action.NotesFileSuffix) ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		log.Default.Debug(ctx, "---\n# Source: %s\n%s\n", filePath, fileContent)
	}

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

func parseLocalLookupResources(paths []string) ([]*unstructured.Unstructured, error) {
	var resources []*unstructured.Unstructured

	seen := make(map[string]bool)

	for _, filePath := range paths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", filePath, err)
		}

		for i, manifest := range releaseutil.SplitManifestsToSlice(string(content)) {
			obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
			if err != nil {
				return nil, fmt.Errorf("decode resource #%d for %q: %w", i+1, filePath, err)
			}

			unstruct := obj.(*unstructured.Unstructured)

			if unstruct.IsList() {
				item := 0

				if err := unstruct.EachListItem(func(o runtime.Object) error {
					res, err := collectLocalLookupResource(o.(*unstructured.Unstructured), seen)
					if err != nil {
						return err
					}

					item++
					resources = append(resources, res)

					return nil
				}); err != nil {
					return nil, fmt.Errorf("collect resource #%d for %q (item %d): %w", i+1, filePath, item, err)
				}

				continue
			}

			res, err := collectLocalLookupResource(unstruct, seen)
			if err != nil {
				return nil, fmt.Errorf("collect resource #%d for %q: %w", i+1, filePath, err)
			}

			resources = append(resources, res)
		}
	}

	return resources, nil
}

func buildChartCapabilities(ctx context.Context, clientFactory kube.ClientFactorier, opts buildChartCapabilitiesOptions) (*chartutil.Capabilities, error) {
	capabilities := &chartutil.Capabilities{
		HelmVersion: chartutil.DefaultCapabilities.HelmVersion,
	}

	if opts.Remote {
		if err := clientFactory.KubeClient().ResetDiscoveryCache(ctx); err != nil {
			return nil, fmt.Errorf("refresh discovery: %w", err)
		}

		kubeVersion, err := clientFactory.KubeClient().ServerVersion(ctx)
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

func buildContextFromJSONSets(jsonSets []string) (map[string]interface{}, error) {
	context := map[string]interface{}{}

	for _, jsonSet := range jsonSets {
		if err := strvals.ParseJSON(jsonSet, context); err != nil {
			return nil, fmt.Errorf("parse JSON set %q: %w", jsonSet, err)
		}
	}

	return context, nil
}

func collectLocalLookupResource(unstruct *unstructured.Unstructured, seen map[string]bool) (*unstructured.Unstructured, error) {
	if unstruct.GetAPIVersion() == "" {
		return nil, fmt.Errorf("apiVersion is missing")
	}

	gvk := unstruct.GroupVersionKind()
	id := spec.IDWithVersion(unstruct.GetName(), unstruct.GetNamespace(), gvk.Group, gvk.Version, gvk.Kind)

	if seen[id] {
		return nil, fmt.Errorf("duplicate resource %s", spec.IDHuman(unstruct.GetName(), unstruct.GetNamespace(), gvk.Group, gvk.Kind))
	}

	seen[id] = true

	return unstruct, nil
}

func isLocalChart(path string) bool {
	return filepath.IsAbs(path) || filepath.HasPrefix(path, "..") || filepath.HasPrefix(path, ".")
}

func renderedTemplatesToResourceSpecs(renderedTemplates map[string]string, releaseNamespace string, opts RenderChartOptions) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec

	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, action.NotesFileSuffix) ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		manifests := releaseutil.SplitManifestsToSlice(fileContent)

		for idx, manifest := range manifests {
			var head releaseutil.SimpleHead
			if err := yaml.UnmarshalWithOptions(
				[]byte(manifest),
				&head,
				// TODO(major): remove
				yaml.AllowDuplicateMapKey(),
			); err != nil {
				return nil, fmt.Errorf("parse YAML resource #%d for %q: %w", idx+1, filePath, err)
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

func validateChart(ctx context.Context, chart *helmchart.Chart) error {
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
