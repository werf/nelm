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

	"github.com/samber/lo"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/yaml"

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

	ChartProvenanceKeyring  string
	ChartProvenanceStrategy string
	ChartRepoNoUpdate       bool
	ChartVersion            string
	ExtraAPIVersions        []string
	HelmOptions             common.HelmOptions
	LocalKubeVersion        string
	NoStandaloneCRDs        bool
	Remote                  bool
	SubchartNotes           bool
	TemplatesAllowDNS       bool
}

type RenderChartResult struct {
	Chart         *v2chart.Chart
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
		Verify:         parseVerificationStrategy(opts.ChartProvenanceStrategy),
		Debug:          log.Default.AcceptLevel(ctx, log.DebugLevel),
		Keyring:        opts.ChartProvenanceKeyring,
		SkipUpdate:     opts.ChartRepoNoUpdate,
		Getters:        getter.Getters(),
		RegistryClient: registryClient,
		// TODO(major): don't read HELM_REPOSITORY_CONFIG anymore
		RepositoryConfig: envOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(major): don't read HELM_REPOSITORY_CACHE anymore
		RepositoryCache:   envOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
		ContentCache:      envOr("HELM_CONTENT_CACHE", helmpath.CachePath("content")),
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

	runtime, err := buildContextFromJSONSets(opts.RuntimeSetJSON)
	if err != nil {
		return nil, fmt.Errorf("build runtime: %w", err)
	}

	log.Default.TraceStruct(ctx, runtime, "Runtime:")

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

	renderedValues, err := chartcommonutil.ToRenderValues(loadedChart, overrideValues, chartcommon.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps)
	if err != nil {
		return nil, fmt.Errorf("build rendered values for chart %q: %w", chartAccessor.Name(), err)
	}

	renderedValues["Runtime"] = runtime

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
		engine = lo.ToPtr(helmengine.Engine{})
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
				if res, err := spec.NewResourceSpecFromManifest(manifest, releaseNamespace, spec.ResourceSpecOptions{
					StoreAs:  common.StoreAsNone,
					FilePath: crd.filename,
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
		var tsChart *v2chart.Chart
		if chartV2 != nil {
			tsChart = chartV2
		} else {
			// TODO(major): refactor to allow native v3 chart handling in TypeScript rendering
			tsChart = convertV3ToV2(chartV3)
		}

		jsRenderedTemplates, err := renderJSTemplates(ctx, chartPath, tsChart, renderedValues)
		if err != nil {
			return nil, fmt.Errorf("render ts chart templates for chart %q: %w", chartAccessor.Name(), err)
		}

		if len(jsRenderedTemplates) > 0 {
			maps.Copy(renderedTemplates, jsRenderedTemplates)
		}
	}

	log.Default.TraceStruct(ctx, renderedTemplates, "Rendered contents of templates/:")

	if r, err := renderedTemplatesToResourceSpecs(renderedTemplates, releaseNamespace, opts); err != nil {
		return nil, fmt.Errorf("convert rendered templates to installable resources for chart at %q: %w", chartPath, err)
	} else {
		resources = append(resources, r...)
	}

	notes := buildChartNotes(chartAccessor.Name(), renderedTemplates, opts.SubchartNotes)

	log.Default.TraceStruct(ctx, notes, "Rendered notes:")

	sort.SliceStable(resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(resources[i], resources[j])
	})

	var resultChart *v2chart.Chart
	if chartV2 != nil {
		resultChart = chartV2
	} else {
		// TODO(major): refactor to allow native v3 chart handling in nelm
		resultChart = convertV3ToV2(chartV3)
	}

	return &RenderChartResult{
		Chart:         resultChart,
		Notes:         notes,
		ReleaseConfig: overrideValues,
		ResourceSpecs: resources,
		Values:        renderedValues.AsMap(),
	}, nil
}

func convertV3ToV2(src *v3chart.Chart) *v2chart.Chart {
	dst := &v2chart.Chart{
		Raw:                src.Raw,
		Templates:          src.Templates,
		Values:             src.Values,
		Schema:             src.Schema,
		SchemaModTime:      src.SchemaModTime,
		Files:              src.Files,
		ModTime:            src.ModTime,
		RuntimeFiles:       src.RuntimeFiles,
		RuntimeDepsFiles:   src.RuntimeDepsFiles,
		ExtraValues:        src.ExtraValues,
		SecretsRuntimeData: src.SecretsRuntimeData,
	}

	if src.Metadata != nil {
		dst.Metadata = convertV3MetadataToV2(src.Metadata)
	}

	if src.Lock != nil {
		dst.Lock = convertV3LockToV2(src.Lock)
	}

	for _, dep := range src.Dependencies() {
		dst.AddDependency(convertV3ToV2(dep))
	}

	return dst
}

func convertV3LockToV2(src *v3chart.Lock) *v2chart.Lock {
	dst := &v2chart.Lock{
		Generated: src.Generated,
		Digest:    src.Digest,
	}

	for _, dependency := range src.Dependencies {
		dst.Dependencies = append(dst.Dependencies, convertV3DependencyToV2(dependency))
	}

	return dst
}

func convertV3MetadataToV2(src *v3chart.Metadata) *v2chart.Metadata {
	dst := &v2chart.Metadata{
		Name:        src.Name,
		Home:        src.Home,
		Sources:     src.Sources,
		Version:     src.Version,
		Description: src.Description,
		Keywords:    src.Keywords,
		Icon:        src.Icon,
		APIVersion:  src.APIVersion,
		Condition:   src.Condition,
		Tags:        src.Tags,
		AppVersion:  src.AppVersion,
		Deprecated:  src.Deprecated,
		Annotations: src.Annotations,
		KubeVersion: src.KubeVersion,
		Type:        src.Type,
	}

	for _, maintainer := range src.Maintainers {
		dst.Maintainers = append(dst.Maintainers, &v2chart.Maintainer{
			Name:  maintainer.Name,
			Email: maintainer.Email,
			URL:   maintainer.URL,
		})
	}

	for _, dependency := range src.Dependencies {
		dst.Dependencies = append(dst.Dependencies, convertV3DependencyToV2(dependency))
	}

	return dst
}

func buildChartCapabilities(ctx context.Context, clientFactory kube.ClientFactorier, opts buildChartCapabilitiesOptions) (*chartcommon.Capabilities, error) {
	capabilities := &chartcommon.Capabilities{
		HelmVersion: chartcommon.DefaultCapabilities.HelmVersion,
	}

	if opts.Remote {
		clientFactory.Discovery().Invalidate()

		kubeVersion, err := clientFactory.Discovery().ServerVersion()
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

func convertV3DependencyToV2(src *v3chart.Dependency) *v2chart.Dependency {
	return &v2chart.Dependency{
		Name:         src.Name,
		Version:      src.Version,
		Repository:   src.Repository,
		Condition:    src.Condition,
		Tags:         src.Tags,
		Enabled:      src.Enabled,
		ImportValues: src.ImportValues,
		Alias:        src.Alias,
	}
}

func envOr(envVar, defaultVal string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}

	return defaultVal
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

func renderJSTemplates(ctx context.Context, chartPath string, chart *v2chart.Chart, renderedValues chartcommon.Values) (map[string]string, error) {
	log.Default.Debug(ctx, "Rendering TypeScript resources for chart %q and its dependencies", chart.Name())

	result, err := ts.RenderChart(ctx, chart, renderedValues)
	if err != nil {
		return nil, fmt.Errorf("render TypeScript: %w", err)
	}

	return result, nil
}

func renderedTemplatesToResourceSpecs(renderedTemplates map[string]string, releaseNamespace string, opts RenderChartOptions) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec

	for filePath, fileContent := range renderedTemplates {
		if strings.HasPrefix(path.Base(filePath), "_") ||
			strings.HasSuffix(filePath, "NOTES.txt") ||
			strings.TrimSpace(fileContent) == "" {
			continue
		}

		manifests := util.SplitManifests(fileContent)

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
