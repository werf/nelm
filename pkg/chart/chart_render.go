package chart

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
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

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/featgate"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	chartv3util "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	chartv2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/cli/values"
	helmdownloader "github.com/werf/nelm/v2/pkg/helm/pkg/downloader"
	helmengine "github.com/werf/nelm/v2/pkg/helm/pkg/engine"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/helmpath"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	releaseutil "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/strvals"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/resource/spec"
	"github.com/werf/nelm/v2/pkg/ts"
	"github.com/werf/nelm/v2/pkg/util"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const setValuesProbeKey = "\x00nelm-probe"

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
	LintMode                        bool
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

// userValues are the values that override the chart values. The precedence, from lowest to highest:
// values.yaml < secret-values.yaml of the chart < the published group (.werf-data) < the deploy
// group (the caller's options).
type userValues struct {
	// ChartSecretValues are the decrypted values from the secret-values.yaml of the chart.
	ChartSecretValues map[string]interface{}
	// DependencyLayers are the publish-time values layers of the werf-published dependencies, below
	// the other layers.
	DependencyLayers []valuesLayer
	Deploy           valuesGroup
	Published        valuesGroup
}

// valuesGroup is one set of value flags. The precedence, from lowest to highest: --values <
// --secret-values < --set* < service values.
type valuesGroup struct {
	// SecretValuesFiles are the decrypted values from --secret-values.
	SecretValuesFiles map[string]interface{}
	// Service are the values injected by werf (ExtraValues), which override all other values of the
	// group.
	Service map[string]interface{}
	// Set are the values from --set*, at the paths they set. A value that patches a lower layer,
	// e.g. replaces a single list element, is the patched value.
	Set map[string]interface{}
	// SetReplacedPaths are the paths at which --set* replaced a map of the lower layers instead of
	// merging into it, e.g. with --set-json.
	SetReplacedPaths [][]string
	// ValuesFiles are the values from --values.
	ValuesFiles map[string]interface{}
}

type valuesLayer struct {
	// AncestorValues are the values of the ancestor charts of the werf-published dependency whose
	// publish-time values the layer has, outside the dependency values. They beat the values that the
	// layer imports into them, like they beat the values imported from the dependency defaults.
	AncestorValues map[string]interface{}
	// ChartValues are the values before export-values and import-values of a layer that is also
	// coalesced with the chart values (see mergeValuesLayers).
	ChartValues map[string]interface{}
	// DependencyPath is the path in the root chart values of the werf-published dependency whose
	// publish-time values the layer has, if any.
	DependencyPath      []string
	IsChartSecretValues bool
	IsService           bool
	// ReplacedPaths are the paths at which the layer replaces the maps of the lower layers instead
	// of merging into them.
	ReplacedPaths [][]string
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

	valuesFilesOpts := &values.Options{
		ValueFiles: opts.ValuesFiles,
	}

	setValuesOpts := &values.Options{
		StringValues:  opts.ValuesSetString,
		Values:        opts.ValuesSet,
		FileValues:    opts.ValuesSetFile,
		JSONValues:    opts.ValuesSetJSON,
		LiteralValues: opts.ValuesSetLiteral,
	}

	log.Default.TraceStruct(ctx, valuesFilesOpts, "Values files options:")
	log.Default.TraceStruct(ctx, setValuesOpts, "Set values options:")
	log.Default.Debug(ctx, "Merging values files for chart at %q", chartPath)

	valuesFilesValues, err := valuesFilesOpts.MergeValues(ctx, getter.Getters())
	if err != nil {
		return nil, fmt.Errorf("merge values files for chart at %q: %w", chartPath, err)
	}

	log.Default.Debug(ctx, "Loading chart at %q", chartPath)

	ctx, loadedChart, publishedData, err := loadChartWithWerfData(ctx, chartPath, &opts.HelmOptions)
	if err != nil {
		return nil, fmt.Errorf("load chart at %q: %w", chartPath, err)
	}

	dependencyLayers, err := foldWerfDataDependencies(ctx, loadedChart, false)
	if err != nil {
		return nil, fmt.Errorf("fold %s of chart at %q dependencies: %w", werfdata.DirName, chartPath, err)
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

	log.Default.Debug(ctx, "Merging override values for chart at %q", chartPath)

	userVals, err := buildUserValues(ctx, setValuesOpts, valuesFilesValues, loadedChart, publishedData)
	if err != nil {
		return nil, fmt.Errorf("merge override values for chart at %q: %w", chartPath, err)
	}

	userVals.DependencyLayers = dependencyLayers

	if publishedData != nil {
		mergePublishedValuesIntoChart(loadedChart, userVals)
	}

	log.Default.TraceStruct(ctx, userVals, "Override values layers:")

	vals, releaseConfig, skippedWarnings, storedChartValues, err := processDependencies(loadedChart, userVals)
	if err != nil {
		return nil, fmt.Errorf("process chart %q dependencies: %w", chartAccessor.Name(), err)
	}

	for _, warning := range skippedWarnings {
		log.Default.Warn(ctx, "%s", warning)
	}

	log.Default.TraceStruct(ctx, loadedChart, "Chart after processing dependencies:")
	log.Default.TraceStruct(ctx, vals, "Merged override values after processing dependencies:")

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

	var rootSetJSON []string
	if publishedData != nil {
		rootSetJSON = append(rootSetJSON, publishedData.Metadata.Values.ValuesSetRootJSON...)
	}

	defaultRootContext, err := buildContextFromJSONSets(append(rootSetJSON, opts.RootSetJSON...))
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

	renderedValues, err := renderValuesToleratingServiceValues(ctx, chartPath, loadedChart, vals, userVals, releaseOptions, caps, opts.NoValuesSchemaValidation)
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
	engine.LintMode = opts.LintMode

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

	setChartValues(loadedChart, storedChartValues)

	return &RenderChartResult{
		Chart:         chartAccessor,
		Notes:         notes,
		ReleaseConfig: releaseConfig,
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
// user values without service values, and the service-free values are validated instead.
// If those pass, the failure was caused only by the injected service values, so the render
// proceeds with the original (service-merged) values and a debug message is logged. If they
// still fail, the service-free error is returned, so the message never names keys the user did
// not write.
//
// The fallback re-loads and re-validates because dependency processing bakes the service values
// into the processed values (and, for v3 charts or v2 charts whose values are invalid without
// them, into the chart's and subcharts' coalesced defaults) before this point, so suppressing them
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
func renderValuesToleratingServiceValues(ctx context.Context, chartPath string, loadedChart helmchart.Charter, vals map[string]interface{}, userVals userValues, releaseOptions chartcommon.ReleaseOptions, caps *chartcommon.Capabilities, skipSchemaValidation bool) (chartcommon.Values, error) {
	renderedValues, err := chartcommonutil.ToRenderValuesWithSchemaValidation(loadedChart, vals, releaseOptions, caps, skipSchemaValidation)
	if err == nil {
		return renderedValues, nil
	}

	if skipSchemaValidation || !chartHasExtraValues(loadedChart) {
		return nil, fmt.Errorf("render values: %w", err)
	}

	serviceFreeChart, serviceFreeVals, loadErr := loadServiceFreeChart(ctx, chartPath, userVals)
	if loadErr != nil {
		return nil, fmt.Errorf("validate service-free values: %w (original schema validation error: %w)", loadErr, err)
	}

	if !sameDependencyGraph(loadedChart, serviceFreeChart) {
		return nil, fmt.Errorf("service values affect which subcharts are enabled, so values cannot be validated without them; re-run with --no-values-schema-validation to skip values schema validation: %w", err)
	}

	if _, serviceFreeErr := chartcommonutil.ToRenderValuesWithSchemaValidation(serviceFreeChart, serviceFreeVals, releaseOptions, caps, false); serviceFreeErr != nil {
		return nil, fmt.Errorf("render values: %w", serviceFreeErr)
	}

	log.Default.Debug(ctx, "Values schema validation failed only for injected service values; tolerating them for chart at %q", chartPath)

	renderedValues, err = chartcommonutil.ToRenderValuesWithSchemaValidation(loadedChart, vals, releaseOptions, caps, true)
	if err != nil {
		return nil, fmt.Errorf("render values: %w", err)
	}

	return renderedValues, nil
}

// loadServiceFreeChart re-loads the chart from disk with its service values (ExtraValues)
// cleared and re-processes its dependencies from the user values without service values, so the
// returned chart and values represent what the user authored without the injected werf/global
// keys. Only infrastructure errors (reload, copy, dependency processing) are returned; schema
// validation of the result is done by the caller.
func loadServiceFreeChart(ctx context.Context, chartPath string, userVals userValues) (helmchart.Charter, map[string]interface{}, error) {
	freshChart, err := loader.Load(ctx, chartPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reload chart at %q: %w", chartPath, err)
	}

	switch c := freshChart.(type) {
	case *v2chart.Chart:
		c.ExtraValues = nil
	case *v3chart.Chart:
		c.ExtraValues = nil
	default:
		return nil, nil, fmt.Errorf("reloaded chart at %q has unexpected type %T", chartPath, freshChart)
	}

	userVals.DependencyLayers, err = foldWerfDataDependencies(ctx, freshChart, true)
	if err != nil {
		return nil, nil, fmt.Errorf("fold %s of reloaded chart at %q dependencies: %w", werfdata.DirName, chartPath, err)
	}

	userVals.Published.Service = nil
	userVals.Deploy.Service = nil

	if len(werfDataFiles(freshChart)) > 0 {
		mergePublishedValuesIntoChart(freshChart, userVals)
	}

	vals, _, _, _, err := processDependencies(freshChart, userVals)
	if err != nil {
		return nil, nil, fmt.Errorf("process reloaded chart at %q dependencies: %w", chartPath, err)
	}

	return freshChart, vals, nil
}

// processDependencies processes the chart dependencies and returns the values to render the chart
// with, the values to store in the release and the warnings about the export-values and
// import-values skipped because of the user values. The values to store in the release are the user
// values with the values exported from them, but without service values, the secret-values.yaml of
// the chart and imported values. The user values are not modified.
// processDependencies processes the dependencies of the chart and returns the values, the values to
// store in the release config, the messages about the skipped imports and exports, and the chart
// values to store in the release.
func processDependencies(chrt helmchart.Charter, userVals userValues) (map[string]interface{}, map[string]interface{}, []string, map[string]interface{}, error) {
	switch c := chrt.(type) {
	case *v2chart.Chart:
		layers := userValuesLayers(userVals)

		conditionLayers, _, err := exportValuesLayers(c, layers)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		vals, err := mergeValuesLayers(conditionLayers, coalescedChartValues(c))
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("merge exported values layers: %w", err)
		}

		if err := chartv2util.ProcessDependenciesWithMergedValues(c, vals); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("process dependencies with merged values: %w", err)
		}

		layers = lo.Filter(layers, func(layer valuesLayer, _ int) bool {
			return layer.DependencyPath == nil || hasDependencyAtPath(c, layer.DependencyPath)
		})

		exportedLayers, skipped, err := exportValuesLayers(c, layers)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		releaseConfig, err := mergeValuesLayers(lo.Filter(exportedLayers, isReleaseConfigLayer), nil)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("merge exported values layers to store in release: %w", err)
		}

		var importedLayers []valuesLayer
		for i, layer := range exportedLayers {
			importedLayerValues, importSkipped, err := chartv2util.ImportUserValues(c, layer.Values, chartv2util.ImportUserValuesOptions{
				ReplacedPaths: higherLayersReplacedPaths(exportedLayers, i),
			})
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("import user values: %w", err)
			}

			if layer.AncestorValues != nil {
				if importedLayerValues, err = deepCopyValues(importedLayerValues); err != nil {
					return nil, nil, nil, nil, fmt.Errorf("copy imported values: %w", err)
				}

				deleteOverriddenValues(importedLayerValues, layer.AncestorValues)
			}

			layer.Values = importedLayerValues
			importedLayers = append(importedLayers, layer)
			skipped = append(skipped, importSkipped...)
		}

		vals, err = mergeValuesLayers(importedLayers, coalescedChartValues(c))
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("merge imported values layers: %w", err)
		}

		storedValues, err := chartValuesWithDependencyLayers(c.Values, importedLayers)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		return vals, releaseConfig, lo.Uniq(skipped), storedValues, nil
	case *v3chart.Chart:
		layers := userValuesLayers(userVals)
		chartVals := coalescedChartValues(c)

		vals, err := mergeValuesLayers(layers, chartVals)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("merge values layers: %w", err)
		}

		releaseConfig, err := mergeValuesLayers(lo.Filter(layers, isReleaseConfigLayer), nil)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("merge values layers to store in release: %w", err)
		}

		if err := chartv3util.ProcessDependenciesWithMergedValues(c, vals); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("process dependencies with merged values: %w", err)
		}

		enabledLayers := lo.Filter(layers, func(layer valuesLayer, _ int) bool {
			return layer.DependencyPath == nil || hasDependencyAtPath(c, layer.DependencyPath)
		})

		if len(enabledLayers) < len(layers) {
			if vals, err = mergeValuesLayers(enabledLayers, chartVals); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("merge values layers of enabled dependencies: %w", err)
			}
		}

		storedValues, err := chartValuesWithDependencyLayers(c.Values, enabledLayers)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		return vals, releaseConfig, nil, storedValues, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("unexpected chart type %T", chrt)
	}
}

// buildUserValues layers the decrypted secret-values.yaml of the chart, the published group built
// from .werf-data, if any, and the deploy group built from the caller's options.
func buildUserValues(ctx context.Context, setValuesOpts *values.Options, valuesFilesValues map[string]interface{}, chrt helmchart.Charter, publishedData *werfdata.Data) (userValues, error) {
	var (
		secretsRuntimeData chartcommon.RuntimeData
		serviceValues      map[string]interface{}
	)

	switch c := chrt.(type) {
	case *v2chart.Chart:
		secretsRuntimeData = c.SecretsRuntimeData
		serviceValues = c.ExtraValues
	case *v3chart.Chart:
		secretsRuntimeData = c.SecretsRuntimeData
		serviceValues = c.ExtraValues
	default:
		return userValues{}, fmt.Errorf("unexpected chart type %T", chrt)
	}

	var chartSecretValues, secretValuesFilesValues map[string]interface{}
	if secretsRuntimeData != nil {
		chartSecretValues = secretsRuntimeData.GetDecryptedDefaultSecretValues()
		secretValuesFilesValues = secretsRuntimeData.GetDecryptedCustomSecretValues()
	}

	var (
		published         valuesGroup
		applyPublishedSet func(base map[string]interface{}) (map[string]interface{}, error)
	)

	if publishedData != nil {
		publishedSecretValues, err := decryptPublishedSecretValues(ctx, publishedData)
		if err != nil {
			return userValues{}, err
		}

		applyPublishedSet, err = publishedSetValuesApplier(ctx, publishedData)
		if err != nil {
			return userValues{}, err
		}

		published, err = buildValuesGroup(ctx, chrt, publishedData.Values, publishedSecretValues, publishedData.ExtraValues, nil, nil, applyPublishedSet)
		if err != nil {
			return userValues{}, fmt.Errorf("build %s values: %w", werfdata.DirName, err)
		}
	}

	setFilesCache := map[string]string{}
	applySet := func(base map[string]interface{}) (map[string]interface{}, error) {
		return setValuesOpts.MergeSetValues(ctx, getter.Getters(), base, values.MergeSetValuesOptions{SetFilesCache: setFilesCache})
	}

	deploy, err := buildValuesGroup(ctx, chrt, valuesFilesValues, secretValuesFilesValues, serviceValues, []valuesGroup{published}, applyPublishedSet, applySet)
	if err != nil {
		return userValues{}, err
	}

	chartSecretValuesLayer, err := deepCopyValues(chartSecretValues)
	if err != nil {
		return userValues{}, fmt.Errorf("copy chart secret values: %w", err)
	}

	return userValues{
		ChartSecretValues: chartSecretValuesLayer,
		Deploy:            deploy,
		Published:         published,
	}, nil
}

// chartValuesWithDependencyLayers returns the processed chart values with the publish-time values
// layers of the werf-published dependencies applied. Stored in the
// release, they keep the precedence of these layers over the export-values and import-values of
// the dependency defaults, so that the release config coalesced with them gives the deployed values.
func chartValuesWithDependencyLayers(chartVals map[string]interface{}, layers []valuesLayer) (map[string]interface{}, error) {
	dependencyLayers := lo.Filter(layers, func(layer valuesLayer, _ int) bool {
		return layer.DependencyPath != nil
	})
	if len(dependencyLayers) == 0 {
		return chartVals, nil
	}

	dependencyVals, err := mergeValuesLayers(dependencyLayers, nil)
	if err != nil {
		return nil, fmt.Errorf("merge dependency values layers: %w", err)
	}

	chartValsCopy, err := deepCopyValues(chartVals)
	if err != nil {
		return nil, fmt.Errorf("copy chart values: %w", err)
	}

	return chartcommonutil.CoalesceTables(dependencyVals, chartValsCopy), nil
}

// buildValuesGroup builds a values group. The --set* values are applied on top of the values files
// and secret values files of the lower groups and of this group, so that they can patch them like in
// Helm, e.g. replace a single list element.
//
// applyLowerSet, if not nil, applies the --set* values of the lower groups before the ones of this
// group, so that they patch the values those set, like on a single command line, e.g. set
// different elements of one list. Only the paths that this group sets are taken from the result.
func buildValuesGroup(ctx context.Context, chrt helmchart.Charter, valuesFilesValues, secretValuesFilesValues, serviceValues map[string]interface{}, lowerGroups []valuesGroup, applyLowerSet, applySet func(base map[string]interface{}) (map[string]interface{}, error)) (valuesGroup, error) {
	var setBaseLayers []map[string]interface{}
	for _, group := range lowerGroups {
		setBaseLayers = append(setBaseLayers, group.ValuesFiles, group.SecretValuesFiles)
	}

	setBaseLayers = append(setBaseLayers, valuesFilesValues, secretValuesFilesValues)

	for i, layer := range setBaseLayers {
		layerCopy, err := deepCopyValues(layer)
		if err != nil {
			return valuesGroup{}, fmt.Errorf("copy values files values: %w", err)
		}

		setBaseLayers[i] = layerCopy
	}

	service, err := deepCopyValues(serviceValues)
	if err != nil {
		return valuesGroup{}, fmt.Errorf("copy service values: %w", err)
	}

	setBase, err := exportedSetValuesBase(chrt, setBaseLayers...)
	if err != nil {
		return valuesGroup{}, err
	}

	applyAllSet := applySet
	if applyLowerSet != nil {
		applyAllSet = func(base map[string]interface{}) (map[string]interface{}, error) {
			base, err := applyLowerSet(base)
			if err != nil {
				return nil, fmt.Errorf("apply lower --set* values: %w", err)
			}

			return applySet(base)
		}
	}

	// The --set* values patch the exported values, e.g. a list element at the child path of an
	// export. The --set* values might be invalid over the exported values though, e.g. when they
	// set a key of a map that is exported as a scalar, then they patch the unexported values.
	withSecrets, err := applyAllSet(setBase)
	if err != nil {
		log.Default.Debug(ctx, "Applying --set* values over the values files values with export-values applied failed, applying them over the values files values: %s", err)

		unexportedSetBase := map[string]interface{}{}
		for _, layer := range setBaseLayers {
			unexportedSetBase = chartcommon.MergeMaps(unexportedSetBase, layer)
		}

		withSecrets, err = applyAllSet(unexportedSetBase)
		if err != nil {
			return valuesGroup{}, fmt.Errorf("merge set values: %w", err)
		}
	}

	setPaths, err := applySet(map[string]interface{}{})
	if err != nil {
		return valuesGroup{}, fmt.Errorf("merge set values: %w", err)
	}

	valuesFilesLayer, err := deepCopyValues(valuesFilesValues)
	if err != nil {
		return valuesGroup{}, fmt.Errorf("copy values files values: %w", err)
	}

	secretValuesFilesLayer, err := deepCopyValues(secretValuesFilesValues)
	if err != nil {
		return valuesGroup{}, fmt.Errorf("copy secret values files values: %w", err)
	}

	return valuesGroup{
		SecretValuesFiles: secretValuesFilesLayer,
		Service:           service,
		Set:               pickValues(setPaths, withSecrets),
		SetReplacedPaths:  setValuesReplacedPaths(setPaths, applySet, nil),
		ValuesFiles:       valuesFilesLayer,
	}, nil
}

func exportValuesLayers(c *v2chart.Chart, layers []valuesLayer) ([]valuesLayer, []string, error) {
	var (
		result  []valuesLayer
		skipped []string
	)

	for i, layer := range layers {
		exportedValues, layerSkipped, err := chartv2util.ExportUserValues(c, layer.Values, chartv2util.ExportUserValuesOptions{
			ReplacedPaths: higherLayersReplacedPaths(layers, i),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("export user values: %w", err)
		}

		layer.Values = exportedValues
		result = append(result, layer)
		skipped = append(skipped, layerSkipped...)
	}

	return result, skipped, nil
}

// mergeValuesLayers merges copies of the values layers, from the lowest to the highest precedence.
// chartVals are the coalesced chart values, or nil if the chart values can't be coalesced without
// user values. The values of a layer with ChartValues that are already applied to the coalesced
// chart values are deleted: merging them again would expose the nulls that coalescing removes, e.g.
// as null keys in subchart values. Without chartVals, only the nulls that are nulls in the
// ChartValues are deleted: coalescing applies them, while the nulls that export-values and
// import-values made might delete chart values that coalescing keeps.
func mergeValuesLayers(layers []valuesLayer, chartVals map[string]interface{}) (map[string]interface{}, error) {
	vals := map[string]interface{}{}
	for _, layer := range layers {
		layerCopy, err := deepCopyValues(layer.Values)
		if err != nil {
			return nil, fmt.Errorf("copy values layer: %w", err)
		}

		switch {
		case layer.ChartValues != nil && chartVals != nil:
			deleteValuesAppliedToChartValues(layerCopy, chartVals)
		case layer.ChartValues != nil:
			deleteNullsOfValues(layerCopy, layer.ChartValues)
		}

		for _, path := range layer.ReplacedPaths {
			deleteValueAtPath(vals, path)
		}

		vals = chartcommon.MergeMaps(vals, layerCopy)
	}

	return vals, nil
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

func userValuesLayers(v userValues) []valuesLayer {
	layers := slices.Clone(v.DependencyLayers)
	layers = append(layers, valuesLayer{ChartValues: v.ChartSecretValues, IsChartSecretValues: true, Values: v.ChartSecretValues})
	layers = append(layers, valuesGroupLayers(v.Published)...)

	return append(layers, valuesGroupLayers(v.Deploy)...)
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

// chartHasExtraValues reports whether the chart has service values, its own or folded into a
// dependency from its .werf-data.
func chartHasExtraValues(chrt helmchart.Charter) bool {
	switch c := chrt.(type) {
	case *v2chart.Chart:
		if len(c.ExtraValues) > 0 {
			return true
		}
	case *v3chart.Chart:
		if len(c.ExtraValues) > 0 {
			return true
		}
	default:
		return false
	}

	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return false
	}

	for _, dep := range acc.Dependencies() {
		if dependencyHasWerfDataExtraValues(dep) {
			return true
		}
	}

	return false
}

// coalescedChartValues returns the chart values coalesced without user values, or nil if the chart
// values are invalid without user values, e.g. when they set a subchart values to a scalar.
func coalescedChartValues(chrt helmchart.Charter) map[string]interface{} {
	vals, err := chartcommonutil.CoalesceValues(chrt, nil)
	if err != nil {
		return nil
	}

	return vals.AsMap()
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

// deleteNullsOfValues deletes the nulls of vals at the paths where origVals have nulls.
func deleteNullsOfValues(vals, origVals map[string]interface{}) {
	for key, origValue := range origVals {
		value, found := vals[key]
		if !found {
			continue
		}

		origTable, origIsTable := origValue.(map[string]interface{})
		table, isTable := value.(map[string]interface{})

		switch {
		case origValue == nil && value == nil:
			delete(vals, key)
		case origIsTable && isTable:
			deleteNullsOfValues(table, origTable)
		}
	}
}

func deleteValueAtPath(vals map[string]interface{}, path []string) {
	for _, key := range path[:len(path)-1] {
		table, ok := vals[key].(map[string]interface{})
		if !ok {
			return
		}

		vals = table
	}

	delete(vals, path[len(path)-1])
}

// deleteValuesAppliedToChartValues deletes the values of vals that the coalesced chart values
// chartVals have, the nulls at the paths that chartVals don't have, and the maps that become empty
// unless they replace a non-map of chartVals.
func deleteValuesAppliedToChartValues(vals, chartVals map[string]interface{}) {
	for key, value := range vals {
		chartValue, found := chartVals[key]
		table, isTable := value.(map[string]interface{})
		chartTable, chartIsTable := chartValue.(map[string]interface{})

		switch {
		case isTable:
			deleteValuesAppliedToChartValues(table, chartTable)

			if len(table) == 0 && chartIsTable {
				delete(vals, key)
			}
		case value == nil && !found, found && reflect.DeepEqual(value, chartValue):
			delete(vals, key)
		}
	}
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

// exportedSetValuesBase returns the values layers merged, each with the export-values of a v2 chart
// applied.
func exportedSetValuesBase(chrt helmchart.Charter, layers ...map[string]interface{}) (map[string]interface{}, error) {
	c, isV2 := chrt.(*v2chart.Chart)

	result := map[string]interface{}{}
	for _, layer := range layers {
		if !isV2 {
			result = chartcommon.MergeMaps(result, layer)
			continue
		}

		exportedLayer, _, err := chartv2util.ExportUserValues(c, layer, chartv2util.ExportUserValuesOptions{})
		if err != nil {
			return nil, fmt.Errorf("export values files values: %w", err)
		}

		result = chartcommon.MergeMaps(result, exportedLayer)
	}

	return result, nil
}

func higherLayersReplacedPaths(layers []valuesLayer, i int) [][]string {
	var paths [][]string
	for _, layer := range layers[i+1:] {
		paths = append(paths, layer.ReplacedPaths...)
	}

	return paths
}

func isLocalChart(path string) bool {
	return filepath.IsAbs(path) || filepath.HasPrefix(path, "..") || filepath.HasPrefix(path, ".")
}

func isReleaseConfigLayer(layer valuesLayer, _ int) bool {
	return !layer.IsChartSecretValues && !layer.IsService && layer.DependencyPath == nil
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

// pickValues returns the values of vals at the paths that are present in keys.
func pickValues(keys, vals map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, keyValue := range keys {
		value, ok := vals[key]
		if !ok {
			continue
		}

		keyTable, keyIsTable := keyValue.(map[string]interface{})

		valueTable, valueIsTable := value.(map[string]interface{})
		if keyIsTable && valueIsTable {
			result[key] = pickValues(keyTable, valueTable)
		} else {
			result[key] = value
		}
	}

	return result
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
			res, err := spec.NewResourceSpecFromManifest(ctx, manifest, releaseNamespace, spec.ResourceSpecOptions{
				FilePath:                        filePath,
				DropInvalidAnnotationsAndLabels: opts.DropInvalidAnnotationsAndLabels,
			})
			if err != nil {
				// Reparse with goccy only to report a better error: it points at the line and
				// column, while the decoder above does not.
				var head releaseutil.SimpleHead
				if parseErr := yaml.UnmarshalWithOptions(
					[]byte(manifest),
					&head,
					yaml.AllowDuplicateMapKey(),
				); parseErr != nil {
					return nil, fmt.Errorf("parse YAML resource #%d for %q: %w", idx+1, filePath, parseErr)
				}

				return nil, fmt.Errorf("construct resource spec for %q: %w", filePath, err)
			}

			resources = append(resources, res)
		}
	}

	return resources, nil
}

// setValuesReplacedPaths returns the paths at which the --set* values replace or delete a map of the
// lower layers instead of merging into it, e.g. with --set-json. setPaths are the --set* values
// applied over no values. For each map path of setPaths, it applies the --set* values over a probe
// map with a probe key at that path: the path is replaced if the probe key is gone, or if the
// --set* values cannot be applied because they set a non-map there before the final map.
func setValuesReplacedPaths(setPaths map[string]interface{}, applySet func(vals map[string]interface{}) (map[string]interface{}, error), path []string) [][]string {
	var replacedPaths [][]string

	for key, value := range setPaths {
		valueTable, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		keyPath := append(slices.Clone(path), key)

		probeTable := map[string]interface{}{setValuesProbeKey: true}

		probe := probeTable
		for i := len(keyPath) - 1; i >= 0; i-- {
			probe = map[string]interface{}{keyPath[i]: probe}
		}

		withSetProbe, err := applySet(probe)
		if err != nil {
			replacedPaths = append(replacedPaths, keyPath)
			continue
		}

		withSetProbeTable := withSetProbe
		for _, pathKey := range keyPath {
			withSetProbeTable, _ = withSetProbeTable[pathKey].(map[string]interface{})
		}

		if _, kept := withSetProbeTable[setValuesProbeKey]; !kept {
			replacedPaths = append(replacedPaths, keyPath)
			continue
		}

		replacedPaths = append(replacedPaths, setValuesReplacedPaths(valueTable, applySet, keyPath)...)
	}

	return replacedPaths
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

func valuesGroupLayers(g valuesGroup) []valuesLayer {
	return []valuesLayer{
		{Values: g.ValuesFiles},
		{Values: g.SecretValuesFiles},
		{ReplacedPaths: g.SetReplacedPaths, Values: g.Set},
		{IsService: true, Values: g.Service},
	}
}
