package chart

import (
	"fmt"
	"io/ioutil"

	helm_v3 "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/werf/errors"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/plan"
)

func NewChartTree(chartPath string, releaseName, releaseNamespace string, revision int, deployType plan.DeployType, actionConfig *action.Configuration, opts NewChartTreeOptions) (*ChartTree, error) {
	var logger log.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = log.NewNullLogger()
	}

	valOpts := &values.Options{
		StringValues: opts.StringValues,
		Values:       opts.Values,
		FileValues:   opts.FileValues,
		ValueFiles:   opts.ValuesFiles,
	}

	getters := getter.All(helm_v3.Settings)

	logger.Debug("Merging values for chart tree at %q ...", chartPath)
	valuesMap, err := valOpts.MergeValues(getters, loader.GlobalLoadOptions.ChartExtender)
	if err != nil {
		return nil, fmt.Errorf("error merging values for chart tree at %q: %w", chartPath, err)
	}
	logger.Debug("Merged values for chart tree at %q", chartPath)

	logger.Debug("Loading chart at %q ...", chartPath)
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, err)
	} else if chart == nil {
		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, action.ErrMissingChart())
	} else if chart.Metadata.Type != "" && chart.Metadata.Type != "application" {
		return nil, errors.NewValidationError("chart %q of type %q can't be deployed", chart.Name(), chart.Metadata.Type)
	} else if chart.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(chart, chart.Metadata.Dependencies); err != nil {
			return nil, fmt.Errorf("error while checking chart dependencies for chart: %w", chart.Name(), err)
		}
	}
	logger.Debug("Loaded chart at %q", chartPath)

	if err := chartutil.ProcessDependencies(chart, &valuesMap); err != nil {
		return nil, fmt.Errorf("error processing chart %q dependencies: %w", chart.Name(), err)
	}

	if chart.Metadata.Deprecated {
		logger.Warn(`Chart "%s:%s" is deprecated`, chart.Name(), chart.Metadata.Version)
	}

	// TODO(ilya-lesikov): allow specifying kube version and additional capabilities manually
	if opts.NoClusterAccess {
		actionConfig.Capabilities = chartutil.DefaultCapabilities.Copy()
		actionConfig.KubeClient = &kubefake.PrintingKubeClient{Out: ioutil.Discard}
		mem := driver.NewMemory()
		mem.SetNamespace(releaseNamespace)
		actionConfig.Releases = storage.Init(mem)
	}

	caps, err := actionConfig.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities for chart %q: %w", chart.Name(), err)
	}

	var isUpgrade bool
	switch deployType {
	case plan.DeployTypeUpgrade, plan.DeployTypeRollback:
		isUpgrade = true
	case plan.DeployTypeInitial, plan.DeployTypeInstall:
		isUpgrade = false
	}

	// FIXME(ilya-lesikov):
	chart.Validate()

	logger.Debug("Rendering values for chart at %q ...", chartPath)
	values, err := chartutil.ToRenderValues(chart, valuesMap, chartutil.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps)
	if err != nil {
		return nil, fmt.Errorf("error building values for chart %q: %w", chart.Name(), err)
	}
	logger.Debug("Rendered values for chart at %q", chartPath)

	logger.Debug("Rendering resources for chart at %q ...", chartPath)
	hooks, manifestsBuf, notes, err := actionConfig.RenderResources(chart, values, "", "", true, false, false, nil, opts.NoClusterAccess, false)
	if err != nil {
		return nil, fmt.Errorf("error rendering resources for chart %q: %w", chart.Name(), err)
	}
	manifests := manifestsBuf.String()
	logger.Debug("Rendered resources for chart at %q", chartPath)

	return &ChartTree{
		preloadedCRDs: chart.CRDObjects(),
		hooks:         hooks,
		resources:     manifests,
		notes:         notes,
		releaseValues: valuesMap,
		values:        values.AsMap(),
		legacyChart:   chart,
		logger:        logger,
	}, nil
}

type NewChartTreeOptions struct {
	Logger          log.Logger
	NoClusterAccess bool
	StringValues    []string
	Values          []string
	FileValues      []string
	ValuesFiles     []string
}

type ChartTree struct {
	preloadedCRDs []chart.CRD
	hooks         []*release.Hook
	resources     string
	notes         string
	releaseValues map[string]interface{}
	values        map[string]interface{}

	legacyChart *chart.Chart

	logger log.Logger
}

func (t *ChartTree) Path() string {
	return t.legacyChart.ChartFullPath()
}

func (t *ChartTree) Name() string {
	return t.legacyChart.Name()
}

// FIXME(ilya-lesikov):
func (t *ChartTree) LegacyPreloadedCRDs() []chart.CRD {
	return t.preloadedCRDs
}

// FIXME(ilya-lesikov):
func (t *ChartTree) LegacyHooks() []*release.Hook {
	return t.hooks
}

// FIXME(ilya-lesikov):
func (t *ChartTree) LegacyResources() string {
	return t.resources
}

// FIXME(ilya-lesikov):
func (t *ChartTree) LegacyChart() *chart.Chart {
	return t.legacyChart
}

func (t *ChartTree) Notes() string {
	return t.notes
}

func (t *ChartTree) ReleaseValues() map[string]interface{} {
	return t.releaseValues
}

func (t *ChartTree) Values() map[string]interface{} {
	return t.values
}
