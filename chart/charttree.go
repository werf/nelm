package chart

import (
	"fmt"
	"strings"
	"sync"

	helm_v3 "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/werf/errors"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/plan"
	"helm.sh/helm/v3/pkg/werf/resourcev2"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

func NewChartTree(chartPath, releaseName, releaseNamespace string, revision int, deployType plan.DeployType, actionConfig *action.Configuration, opts NewChartTreeOptions) *ChartTree {
	var logger log.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = log.NewNullLogger()
	}

	return &ChartTree{
		chartPath:        chartPath,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
		revision:         revision,
		deployType:       deployType,
		actionConfig:     actionConfig,
		stringSetValues:  opts.StringSetValues,
		setValues:        opts.SetValues,
		fileValues:       opts.FileValues,
		valuesFiles:      opts.ValuesFiles,
		mapper:           opts.Mapper,
		discoveryClient:  opts.DiscoveryClient,
		logger:           logger,
	}
}

type NewChartTreeOptions struct {
	Logger          log.Logger
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
	StringSetValues []string
	SetValues       []string
	FileValues      []string
	ValuesFiles     []string
}

type ChartTree struct {
	chartPath        string
	releaseName      string
	releaseNamespace string
	revision         int
	deployType       plan.DeployType
	actionConfig     *action.Configuration
	stringSetValues  []string
	setValues        []string
	fileValues       []string
	valuesFiles      []string
	mapper           meta.ResettableRESTMapper
	discoveryClient  discovery.CachedDiscoveryInterface
	logger           log.Logger

	loadOnce sync.Once

	standaloneCRDs          []*resourcev2.LocalStandaloneCRD
	hookCRDs                []*resourcev2.LocalHookCRD
	hookResources           []*resourcev2.LocalHookResource
	generalCRDs             []*resourcev2.LocalGeneralCRD
	generalResources        []*resourcev2.LocalGeneralResource
	standaloneCRDsManifests []string
	hookManifests           []string
	generalManifests        []string
	notes                   string
	releaseValues           map[string]interface{}
	finalValues             map[string]interface{}
	legacyChart             *chart.Chart
}

func (t *ChartTree) Load() (*ChartTree, error) {
	var err error
	t.loadOnce.Do(func() {
		err = t.load()
	})

	return t, err
}

func (t *ChartTree) load() error {
	valOpts := &values.Options{
		StringValues: t.stringSetValues,
		Values:       t.setValues,
		FileValues:   t.fileValues,
		ValueFiles:   t.valuesFiles,
	}

	getters := getter.All(helm_v3.Settings)

	var err error
	t.logger.Debug("Merging values for chart tree at %q ...", t.chartPath)
	t.releaseValues, err = valOpts.MergeValues(getters, loader.GlobalLoadOptions.ChartExtender)
	if err != nil {
		return fmt.Errorf("error merging values for chart tree at %q: %w", t.chartPath, err)
	}
	t.logger.Debug("Merged values for chart tree at %q", t.chartPath)

	t.logger.Debug("Loading chart at %q ...", t.chartPath)
	t.legacyChart, err = loader.Load(t.chartPath)
	if err != nil {
		return fmt.Errorf("error loading chart for chart tree at %q: %w", t.chartPath, err)
	} else if t.legacyChart == nil {
		return fmt.Errorf("error loading chart for chart tree at %q: %w", t.chartPath, action.ErrMissingChart())
	} else if t.legacyChart.Metadata.Type != "" && t.legacyChart.Metadata.Type != "application" {
		return errors.NewValidationError("chart %q of type %q can't be deployed", t.legacyChart.Name(), t.legacyChart.Metadata.Type)
	} else if t.legacyChart.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(t.legacyChart, t.legacyChart.Metadata.Dependencies); err != nil {
			return fmt.Errorf("error while checking chart dependencies for chart: %w", t.legacyChart.Name(), err)
		}
	}
	t.logger.Debug("Loaded chart at %q", t.chartPath)

	if err := chartutil.ProcessDependencies(t.legacyChart, &t.releaseValues); err != nil {
		return fmt.Errorf("error processing chart %q dependencies: %w", t.legacyChart.Name(), err)
	}

	if t.legacyChart.Metadata.Deprecated {
		t.logger.Warn(`Chart "%s:%s" is deprecated`, t.legacyChart.Name(), t.legacyChart.Metadata.Version)
	}

	caps, err := t.actionConfig.GetCapabilities()
	if err != nil {
		return fmt.Errorf("error getting capabilities for chart %q: %w", t.legacyChart.Name(), err)
	}

	var isUpgrade bool
	switch t.deployType {
	case plan.DeployTypeUpgrade, plan.DeployTypeRollback:
		isUpgrade = true
	case plan.DeployTypeInitial, plan.DeployTypeInstall:
		isUpgrade = false
	}

	// FIXME(ilya-lesikov): already done?
	t.legacyChart.Validate()

	t.logger.Debug("Rendering values for chart at %q ...", t.chartPath)
	values, err := chartutil.ToRenderValues(t.legacyChart, t.releaseValues, chartutil.ReleaseOptions{
		Name:      t.releaseName,
		Namespace: t.releaseNamespace,
		Revision:  t.revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps)
	if err != nil {
		return fmt.Errorf("error building values for chart %q: %w", t.legacyChart.Name(), err)
	}
	t.logger.Debug("Rendered values for chart at %q", t.chartPath)

	t.finalValues = values.AsMap()
	noClusterAccess := t.mapper == nil || t.discoveryClient == nil

	t.logger.Debug("Rendering resources for chart at %q ...", t.chartPath)
	hooks, manifestsBuf, notes, err := t.actionConfig.RenderResources(t.legacyChart, values, "", "", true, false, false, nil, noClusterAccess, false)
	if err != nil {
		return fmt.Errorf("error rendering resources for chart %q: %w", t.legacyChart.Name(), err)
	}
	t.logger.Debug("Rendered resources for chart at %q", t.chartPath)
	manifests := manifestsBuf.String()
	t.notes = notes

	for _, crd := range t.legacyChart.CRDObjects() {
		for _, manifest := range releaseutil.SplitManifests(string(crd.File.Data)) {
			if !strings.HasPrefix(manifest, "# Source: ") {
				manifest = "# Source: " + crd.Filename + "\n" + manifest
			}
			t.standaloneCRDsManifests = append(t.standaloneCRDsManifests, manifest)
		}
	}

	for _, hook := range hooks {
		for _, manifest := range releaseutil.SplitManifests(hook.Manifest) {
			t.hookManifests = append(t.hookManifests, manifest)
		}
	}

	for _, manifest := range releaseutil.SplitManifests(manifests) {
		t.generalManifests = append(t.generalManifests, manifest)
	}

	if t.standaloneCRDs, err = resourcev2.BuildLocalStandaloneCRDsFromManifests(t.standaloneCRDsManifests, resourcev2.BuildLocalStandaloneCRDsFromManifestsOptions{
		Mapper:          t.mapper,
		DiscoveryClient: t.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building standalone CRDs for chart at %q: %w", t.chartPath, err)
	}

	if t.hookCRDs, err = resourcev2.BuildLocalHookCRDsFromManifests(t.hookManifests, resourcev2.BuildLocalHookCRDsFromManifestsOptions{
		Mapper:          t.mapper,
		DiscoveryClient: t.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building hook CRDs for chart at %q: %w", t.chartPath, err)
	}

	if t.hookResources, err = resourcev2.BuildLocalHookResourcesFromManifests(t.hookManifests, resourcev2.BuildLocalHookResourcesFromManifestsOptions{
		Mapper:          t.mapper,
		DiscoveryClient: t.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building hook resources for chart at %q: %w", t.chartPath, err)
	}

	if t.generalCRDs, err = resourcev2.BuildLocalGeneralCRDsFromManifests(t.generalManifests, resourcev2.BuildLocalGeneralCRDsFromManifestsOptions{
		Mapper:          t.mapper,
		DiscoveryClient: t.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building general CRDs for chart at %q: %w", t.chartPath, err)
	}

	if t.generalResources, err = resourcev2.BuildLocalGeneralResourcesFromManifests(t.generalManifests, resourcev2.BuildLocalGeneralResourcesFromManifestsOptions{
		Mapper:          t.mapper,
		DiscoveryClient: t.discoveryClient,
	}); err != nil {
		return fmt.Errorf("error building general resources for chart at %q: %w", t.chartPath, err)
	}

	return nil
}

func (t *ChartTree) Path() string {
	return t.legacyChart.ChartFullPath()
}

func (t *ChartTree) Name() string {
	return t.legacyChart.Name()
}

func (t *ChartTree) LegacyChart() *chart.Chart {
	return t.legacyChart
}

func (t *ChartTree) StandaloneCRDs() []*resourcev2.LocalStandaloneCRD {
	return t.standaloneCRDs
}

func (t *ChartTree) HookCRDs() []*resourcev2.LocalHookCRD {
	return t.hookCRDs
}

func (t *ChartTree) HookResources() []*resourcev2.LocalHookResource {
	return t.hookResources
}

func (t *ChartTree) GeneralCRDs() []*resourcev2.LocalGeneralCRD {
	return t.generalCRDs
}

func (t *ChartTree) GeneralResources() []*resourcev2.LocalGeneralResource {
	return t.generalResources
}

func (t *ChartTree) StandaloneCRDsManifests() []string {
	return t.standaloneCRDsManifests
}

func (t *ChartTree) HookManifests() []string {
	return t.hookManifests
}

func (t *ChartTree) GeneralManifests() []string {
	return t.generalManifests
}

func (t *ChartTree) Notes() string {
	return t.notes
}

func (t *ChartTree) ReleaseValues() map[string]interface{} {
	return t.releaseValues
}

func (t *ChartTree) FinalValues() map[string]interface{} {
	return t.finalValues
}
