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
	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	v3chart "github.com/werf/nelm/pkg/helm/intern/chart/v3"
	chartv3util "github.com/werf/nelm/pkg/helm/intern/chart/v3/util"
	"github.com/werf/nelm/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/pkg/helm/pkg/chart/common/util"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	chartv2util "github.com/werf/nelm/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/pkg/helm/pkg/cli/values"
	helmdownloader "github.com/werf/nelm/pkg/helm/pkg/downloader"
	helmengine "github.com/werf/nelm/pkg/helm/pkg/engine"
	"github.com/werf/nelm/pkg/helm/pkg/getter"
	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/helm/pkg/registry"
	releaseutil "github.com/werf/nelm/pkg/helm/pkg/release/v1/util"
	"github.com/werf/nelm/pkg/helm/pkg/strvals"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resource/spec"
	"github.com/werf/nelm/pkg/ts"
	"github.com/werf/nelm/pkg/util"
)

type RenderChartOptions struct {
	common.ChartRepoConnectionOptions
	common.ValuesOptions

	ChartProvenanceKeyring          string
	ChartProvenanceStrategy         string
	ChartRepoNoUpdate               bool
	ChartVersion                    string
	DenoBinaryPath                  string
	DropInvalidAnnotationsAndLabels bool
	ExtraAPIVersions                []string
	HelmOptions                     common.HelmOptions
	IgnoreBundleJS                  bool
	LocalKubeVersion                string
	LocalLookupResourcesPaths       []string
	NoStandaloneCRDs                bool
	NoValuesSchemaValidation        bool
	Remote                          bool
	SubchartNotes                   bool
	TempDirPath                     string
	TemplatesAllowDNS               bool
}

type RenderChartResult struct {
	Chart         helmchart.Accessor
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
		Out:               os.Stdout,
		ChartPath:         chartPath,
		Verify:            parseVerificationStrategy(opts.ChartProvenanceStrategy),
		Debug:             log.Default.AcceptLevel(ctx, log.DebugLevel),
		Keyring:           opts.ChartProvenanceKeyring,
		SkipUpdate:        opts.ChartRepoNoUpdate,
		Getters:           getter.Getters(),
		RegistryClient:    registryClient,
		RepositoryConfig:  helmpath.ConfigPath("repositories.yaml"),
		RepositoryCache:   helmpath.CachePath("repository"),
		ContentCache:      helmpath.CachePath("content"),
		AllowMissingRepos: true,
	}

	opts.HelmOptions.ChartLoadOpts.ChartDepsDownloader = depDownloader

	ctx = common.ContextWithHelmOptions(ctx, opts.HelmOptions)

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

	overrideValues, err := overrideValuesOpts.MergeValues(ctx, getter.Getters())
	if err != nil {
		return nil, fmt.Errorf("merge override values for chart at %q: %w", chartPath, err)
	}

	log.Default.TraceStruct(ctx, overrideValues, "Merged override values:")
	log.Default.Debug(ctx, "Loading chart at %q", chartPath)

	loadedChart, err := loader.Load(ctx, chartPath)
	if err != nil {
		return nil, fmt.Errorf("load chart at %q: %w", chartPath, err)
	}

	var (
		chartV2 *v2chart.Chart
		chartV3 *v3chart.Chart
	)

	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		chartV2 = c
	case *v3chart.Chart:
		chartV3 = c
	default:
		return nil, fmt.Errorf("loaded chart has unexpected type %T", loadedChart)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	if err := validateChart(ctx, loadedChart, chartAccessor); err != nil {
		return nil, fmt.Errorf("validate chart at %q: %w", chartPath, err)
	}

	log.Default.TraceStruct(ctx, loadedChart, "Chart:")

	pristineOverrideValues, err := deepCopyValues(overrideValues)
	if err != nil {
		return nil, fmt.Errorf("copy override values for chart %q: %w", chartAccessor.Name(), err)
	}

	if chartV2 != nil {
		if err := chartv2util.ProcessDependencies(chartV2, &overrideValues); err != nil {
			return nil, fmt.Errorf("process chart %q dependencies: %w", chartV2.Name(), err)
		}
	} else {
		if err := chartv3util.ProcessDependencies(chartV3, overrideValues); err != nil {
			return nil, fmt.Errorf("process chart %q dependencies: %w", chartV3.Name(), err)
		}
	}

	log.Default.TraceStruct(ctx, loadedChart, "Chart after processing dependencies:")
	log.Default.TraceStruct(ctx, overrideValues, "Merged override values after processing dependencies:")

	var chartKubeVersion string
	if chartV2 != nil {
		chartKubeVersion = chartV2.Metadata.KubeVersion
	} else {
		chartKubeVersion = chartV3.Metadata.KubeVersion
	}

	caps, err := buildChartCapabilities(ctx, clientFactory, buildChartCapabilitiesOptions{
		ExtraAPIVersions: opts.ExtraAPIVersions,
		LocalKubeVersion: opts.LocalKubeVersion,
		Remote:           opts.Remote,
	})
	if err != nil {
		return nil, fmt.Errorf("build capabilities for chart %q: %w", chartAccessor.Name(), err)
	}

	log.Default.TraceStruct(ctx, caps, "Capabilities:")

	if chartKubeVersion != "" && !chartv2util.IsCompatibleRange(chartKubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", chartKubeVersion, caps.KubeVersion.String())
	}

	defaultRootContext, err := buildContextFromJSONSets(opts.RootSetJSON)
	if err != nil {
		return nil, fmt.Errorf("build default root context: %w", err)
	}

	log.Default.TraceStruct(ctx, defaultRootContext, "Default root context:")

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

	releaseOptions := chartcommon.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}

	renderedValues, err := renderValuesToleratingServiceValues(ctx, chartPath, loadedChart, overrideValues, pristineOverrideValues, releaseOptions, caps, opts.NoValuesSchemaValidation)
	if err != nil {
		return nil, fmt.Errorf("build rendered values for chart %q: %w", chartAccessor.Name(), err)
	}

	for k, v := range defaultRootContext {
		if _, exists := renderedValues[k]; !exists {
			renderedValues[k] = v
		}
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

			engine.SetClientProvider(newLocalClientProvider(localLookupResources))
		}
	}

	engine.EnableDNS = opts.TemplatesAllowDNS

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)

	var resources []*spec.ResourceSpec

	if !opts.NoStandaloneCRDs {
		type crdRef struct {
			data     []byte
			filename string
		}

		var crds []crdRef

		if chartV2 != nil {
			for _, crd := range chartV2.CRDObjects() {
				crds = append(crds, crdRef{data: crd.File.Data, filename: crd.Filename})
			}
		} else {
			for _, crd := range chartV3.CRDObjects() {
				crds = append(crds, crdRef{data: crd.File.Data, filename: crd.Filename})
			}
		}

		for _, crd := range crds {
			for _, manifest := range util.SplitManifests(string(crd.data)) {
				if res, err := spec.NewResourceSpecFromManifest(ctx, manifest, releaseNamespace, spec.ResourceSpecOptions{
					StoreAs:                         common.StoreAsNone,
					FilePath:                        crd.filename,
					DropInvalidAnnotationsAndLabels: opts.DropInvalidAnnotationsAndLabels,
				}); err != nil {
					return nil, fmt.Errorf("construct standalone CRD for chart at %q: %w", chartPath, err)
				} else {
					resources = append(resources, res)
				}
			}
		}
	}

	renderedTemplates, err := engine.Render(ctx, loadedChart, renderedValues)
	if err != nil {
		return nil, fmt.Errorf("render resources for chart %q: %w", chartAccessor.Name(), err)
	}

	if featgate.FeatGateTypescript.Enabled() {
		log.Default.Debug(ctx, "Rendering TypeScript resources for chart %q and its dependencies", chartAccessor.Name())

		jsRenderedTemplates, err := ts.RenderChart(ctx, chartAccessor, renderedValues, opts.IgnoreBundleJS, chartPath, opts.TempDirPath, opts.DenoBinaryPath)
		if err != nil {
			return nil, fmt.Errorf("render TypeScript templates for chart %q: %w", chartAccessor.Name(), err)
		}

		if len(jsRenderedTemplates) > 0 {
			maps.Copy(renderedTemplates, jsRenderedTemplates)
		}
	}

	log.Default.Debug(ctx, "Rendered content:")

	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, "NOTES.txt") ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		log.Default.Debug(ctx, "---\n# Source: %s\n%s\n", filePath, fileContent)
	}

	if r, err := renderedTemplatesToResourceSpecs(ctx, renderedTemplates, releaseNamespace, opts); err != nil {
		return nil, fmt.Errorf("convert rendered templates to installable resources for chart at %q: %w", chartPath, err)
	} else {
		resources = append(resources, r...)
	}

	notes := buildChartNotes(chartAccessor.Name(), renderedTemplates, opts.SubchartNotes)

	log.Default.TraceStruct(ctx, notes, "Rendered notes:")

	sort.SliceStable(resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(resources[i], resources[j])
	})

	return &RenderChartResult{
		Chart:         chartAccessor,
		Notes:         notes,
		ReleaseConfig: overrideValues,
		ResourceSpecs: resources,
		Values:        renderedValues.AsMap(),
	}, nil
}

// renderValuesToleratingServiceValues coalesces and schema-validates the chart values,
// tolerating the werf/nelm service values (werf, global, dockerconfigjson) that are injected
// into every chart via ExtraValues. A chart whose values.schema.json rejects unknown keys
// (e.g. additionalProperties:false) would otherwise fail validation solely because of those
// injected keys, even when the user's own values are valid — the regression fixed here.
//
// On a validation failure, and only when the chart actually carries service values, the chart
// is re-loaded from disk with ExtraValues cleared, its dependencies are re-processed from the
// pristine (pre-processing) override values, and the service-free values are validated instead.
// If those pass, the failure was caused only by the injected service values, so the render
// proceeds with the original (service-merged) values and a debug message is logged. If they
// still fail, the service-free error is returned, so the message never names keys the user did
// not write.
//
// The fallback re-loads and re-validates because ProcessDependencies bakes the service values
// into the chart's (and subcharts') coalesced defaults before this point, so suppressing them
// after the fact is not possible without re-deriving from a pristine chart state.
//
// The fallback is only trusted when the service-free chart resolves the SAME dependency graph
// as the rendered one: dependency conditions are evaluated against coalesced values that
// include the service values (e.g. a `condition: werf.is_stub`), so clearing them could enable
// or disable a subchart and make the validated graph differ from the rendered one. When the
// graphs differ, the fallback is refused and the original error is returned pointing at the
// explicit escape hatch.
//
// The accept path re-coalesces the original chart rather than reusing the first result. This
// is exact for the service values werf injects, which never contain explicit nil leaves; a nil
// leaf inside ExtraValues could in theory make the re-coalesce diverge from the first one, but
// werf's GetServiceValues never produces one.
func renderValuesToleratingServiceValues(ctx context.Context, chartPath string, loadedChart helmchart.Charter, overrideValues, pristineOverrideValues map[string]interface{}, releaseOptions chartcommon.ReleaseOptions, caps *chartcommon.Capabilities, skipSchemaValidation bool) (chartcommon.Values, error) {
	renderedValues, err := chartcommonutil.ToRenderValuesWithSchemaValidation(loadedChart, overrideValues, releaseOptions, caps, skipSchemaValidation)
	if err == nil {
		return renderedValues, nil
	}

	if skipSchemaValidation || !chartHasExtraValues(loadedChart) {
		return nil, fmt.Errorf("render values: %w", err)
	}

	serviceFreeChart, serviceFreeOverrideValues, loadErr := loadServiceFreeChart(ctx, chartPath, pristineOverrideValues)
	if loadErr != nil {
		return nil, fmt.Errorf("validate service-free values: %w (original schema validation error: %w)", loadErr, err)
	}

	if !sameDependencyGraph(loadedChart, serviceFreeChart) {
		return nil, fmt.Errorf("service values affect which subcharts are enabled, so values cannot be validated without them; re-run with --no-values-schema-validation to skip values schema validation: %w", err)
	}

	if _, serviceFreeErr := chartcommonutil.ToRenderValuesWithSchemaValidation(serviceFreeChart, serviceFreeOverrideValues, releaseOptions, caps, false); serviceFreeErr != nil {
		return nil, fmt.Errorf("render values: %w", serviceFreeErr)
	}

	log.Default.Debug(ctx, "Values schema validation failed only for injected service values; tolerating them for chart at %q", chartPath)

	renderedValues, err = chartcommonutil.ToRenderValuesWithSchemaValidation(loadedChart, overrideValues, releaseOptions, caps, true)
	if err != nil {
		return nil, fmt.Errorf("render values: %w", err)
	}

	return renderedValues, nil
}

// loadServiceFreeChart re-loads the chart from disk with its service values (ExtraValues)
// cleared and re-processes its dependencies from the pristine override values, so the returned
// chart and override values represent what the user authored without the injected werf/global
// keys. Only infrastructure errors (reload, copy, dependency processing) are returned; schema
// validation of the result is done by the caller.
func loadServiceFreeChart(ctx context.Context, chartPath string, pristineOverrideValues map[string]interface{}) (helmchart.Charter, map[string]interface{}, error) {
	freshChart, err := loader.Load(ctx, chartPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reload chart at %q: %w", chartPath, err)
	}

	overrideValues, err := deepCopyValues(pristineOverrideValues)
	if err != nil {
		return nil, nil, fmt.Errorf("copy override values for chart at %q: %w", chartPath, err)
	}

	switch c := freshChart.(type) {
	case *v2chart.Chart:
		c.ExtraValues = nil
		if err := chartv2util.ProcessDependencies(c, &overrideValues); err != nil {
			return nil, nil, fmt.Errorf("process chart %q dependencies: %w", c.Name(), err)
		}
	case *v3chart.Chart:
		c.ExtraValues = nil
		if err := chartv3util.ProcessDependencies(c, overrideValues); err != nil {
			return nil, nil, fmt.Errorf("process chart %q dependencies: %w", c.Name(), err)
		}
	default:
		return nil, nil, fmt.Errorf("reloaded chart at %q has unexpected type %T", chartPath, freshChart)
	}

	return freshChart, overrideValues, nil
}

func parseLocalLookupResources(paths []string) ([]*unstructured.Unstructured, error) {
	var resources []*unstructured.Unstructured

	seen := make(map[string]bool)

	for _, filePath := range paths {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", filePath, err)
		}

		for i, manifest := range util.SplitManifestsKeepingEmpty(string(content)) {
			if manifest == "" {
				continue
			}

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
					return nil, fmt.Errorf("collect resource #%d for %q (item %d): %w", i+1, filePath, item+1, err)
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

// sameDependencyGraph reports whether two processed charts resolve to the same set of enabled
// subcharts (compared by each node's full chart path, recursively).
func sameDependencyGraph(a, b helmchart.Charter) bool {
	sigA, err := dependencyGraphSignature(a)
	if err != nil {
		return false
	}

	sigB, err := dependencyGraphSignature(b)
	if err != nil {
		return false
	}

	if len(sigA) != len(sigB) {
		return false
	}

	for i := range sigA {
		if sigA[i] != sigB[i] {
			return false
		}
	}

	return true
}

func buildChartCapabilities(ctx context.Context, clientFactory kube.ClientFactorier, opts buildChartCapabilitiesOptions) (*chartcommon.Capabilities, error) {
	capabilities := &chartcommon.Capabilities{
		HelmVersion: chartcommon.DefaultCapabilities.HelmVersion,
	}

	if opts.Remote {
		if err := clientFactory.KubeClient().ResetDiscoveryCache(ctx); err != nil {
			return nil, fmt.Errorf("refresh discovery: %w", err)
		}

		kubeVersion, err := clientFactory.KubeClient().ServerVersion(ctx)
		if err != nil {
			return nil, fmt.Errorf("get kubernetes server version: %w", err)
		}

		capabilities.KubeVersion = chartcommon.KubeVersion{
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
			kubeVersion, err := chartcommon.ParseKubeVersion(opts.LocalKubeVersion)
			if err != nil {
				return nil, fmt.Errorf("parse kube version %q: %w", opts.LocalKubeVersion, err)
			}

			capabilities.KubeVersion = *kubeVersion
		} else {
			capabilities.KubeVersion = chartcommon.DefaultCapabilities.KubeVersion
		}

		capabilities.APIVersions = chartcommon.DefaultCapabilities.APIVersions
	}

	if opts.ExtraAPIVersions != nil {
		capabilities.APIVersions = append(capabilities.APIVersions, chartcommon.VersionSet(opts.ExtraAPIVersions)...)
	}

	return capabilities, nil
}

func buildChartNotes(chartName string, renderedTemplates map[string]string, renderSubchartNotes bool) string {
	var resultBuf bytes.Buffer

	for filePath, fileContent := range renderedTemplates {
		if !strings.HasSuffix(filePath, "NOTES.txt") {
			continue
		}

		fileContent = strings.TrimRightFunc(fileContent, unicode.IsSpace)
		if fileContent == "" {
			continue
		}

		isTopLevelNotes := filePath == path.Join(chartName, "templates", "NOTES.txt")

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

func chartHasExtraValues(chrt helmchart.Charter) bool {
	switch c := chrt.(type) {
	case *v2chart.Chart:
		return len(c.ExtraValues) > 0
	case *v3chart.Chart:
		return len(c.ExtraValues) > 0
	default:
		return false
	}
}

func collectLocalLookupResource(unstruct *unstructured.Unstructured, seen map[string]bool) (*unstructured.Unstructured, error) {
	if unstruct.GetAPIVersion() == "" {
		return nil, fmt.Errorf("apiVersion is missing")
	}

	if unstruct.GetName() == "" {
		return nil, fmt.Errorf("name is missing")
	}

	gvk := unstruct.GroupVersionKind()
	id := spec.IDWithVersion(unstruct.GetName(), unstruct.GetNamespace(), gvk.Group, gvk.Version, gvk.Kind)

	if seen[id] {
		return nil, fmt.Errorf("duplicate resource %s", spec.IDHuman(unstruct.GetName(), unstruct.GetNamespace(), gvk.Group, gvk.Kind))
	}

	seen[id] = true

	return unstruct, nil
}

func deepCopyValues(vals map[string]interface{}) (map[string]interface{}, error) {
	if vals == nil {
		return map[string]interface{}{}, nil
	}

	copied, err := copystructure.Copy(vals)
	if err != nil {
		return nil, fmt.Errorf("deep copy values: %w", err)
	}

	return copied.(map[string]interface{}), nil
}

func dependencyGraphSignature(chrt helmchart.Charter) ([]string, error) {
	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	var paths []string
	for _, dep := range acc.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("create dependency accessor: %w", err)
		}

		paths = append(paths, depAcc.ChartFullPath())

		subPaths, err := dependencyGraphSignature(dep)
		if err != nil {
			return nil, err
		}

		paths = append(paths, subPaths...)
	}

	sort.Strings(paths)

	return paths, nil
}

func isLocalChart(path string) bool {
	return filepath.IsAbs(path) || filepath.HasPrefix(path, "..") || filepath.HasPrefix(path, ".")
}

func parseVerificationStrategy(s string) helmdownloader.VerificationStrategy {
	switch s {
	case "verify":
		return helmdownloader.VerifyAlways
	case "verify-if-possible":
		return helmdownloader.VerifyIfPossible
	case "later":
		return helmdownloader.VerifyLater
	default:
		return helmdownloader.VerifyNever
	}
}

func renderedTemplatesToResourceSpecs(ctx context.Context, renderedTemplates map[string]string, releaseNamespace string, opts RenderChartOptions) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec

	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, "NOTES.txt") ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		manifests := util.SplitManifests(fileContent)

		for idx, manifest := range manifests {
			var head releaseutil.SimpleHead
			if err := yaml.UnmarshalWithOptions(
				[]byte(manifest),
				&head,
				yaml.AllowDuplicateMapKey(),
			); err != nil {
				return nil, fmt.Errorf("parse YAML resource #%d for %q: %w", idx+1, filePath, err)
			}

			if res, err := spec.NewResourceSpecFromManifest(ctx, manifest, releaseNamespace, spec.ResourceSpecOptions{
				FilePath:                        filePath,
				DropInvalidAnnotationsAndLabels: opts.DropInvalidAnnotationsAndLabels,
			}); err != nil {
				return nil, fmt.Errorf("construct resource spec for %q: %w", filePath, err)
			} else {
				resources = append(resources, res)
			}
		}
	}

	return resources, nil
}

func validateChart(ctx context.Context, chrt helmchart.Charter, acc helmchart.Accessor) error {
	if chrt == nil {
		return fmt.Errorf("load chart: missing chart")
	}

	meta := acc.MetadataAsMap()

	chartType, _ := meta["Type"].(string)
	if chartType != "" && chartType != "application" {
		return fmt.Errorf("chart %q of type %q can't be deployed", acc.Name(), chartType)
	}

	if metaDeps := acc.MetaDependencies(); len(metaDeps) > 0 {
		if err := action.CheckDependencies(chrt, metaDeps); err != nil {
			return fmt.Errorf("check chart dependencies for chart %q: %w", acc.Name(), err)
		}
	}

	if acc.Deprecated() {
		chartVersion, _ := meta["Version"].(string)
		log.Default.Warn(ctx, `Chart "%s:%s" is deprecated`, acc.Name(), chartVersion)
	}

	return nil
}
