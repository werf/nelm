package chrttree

import (
	"context"
	"fmt"

	helm_v3 "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/releaseutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resrc"
)

func NewChartTree(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, actionConfig *action.Configuration, opts ChartTreeOptions) (*ChartTree, error) {
	valOpts := &values.Options{
		StringValues: opts.StringSetValues,
		Values:       opts.SetValues,
		FileValues:   opts.FileValues,
		ValueFiles:   opts.ValuesFiles,
	}

	getters := getter.All(helm_v3.Settings)

	log.Default.Debug(ctx, "Merging values for chart tree at %q", chartPath)
	releaseValues, err := valOpts.MergeValues(getters, loader.GlobalLoadOptions.ChartExtender)
	if err != nil {
		return nil, fmt.Errorf("error merging values for chart tree at %q: %w", chartPath, err)
	}

	log.Default.Debug(ctx, "Loading chart at %q", chartPath)
	legacyChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, err)
	} else if legacyChart == nil {
		return nil, fmt.Errorf("error loading chart for chart tree at %q: %w", chartPath, action.ErrMissingChart())
	} else if legacyChart.Metadata.Type != "" && legacyChart.Metadata.Type != "application" {
		return nil, fmt.Errorf("chart %q of type %q can't be deployed", legacyChart.Name(), legacyChart.Metadata.Type)
	} else if legacyChart.Metadata.Dependencies != nil {
		if err := action.CheckDependencies(legacyChart, legacyChart.Metadata.Dependencies); err != nil {
			return nil, fmt.Errorf("error while checking chart dependencies for chart %q: %w", legacyChart.Name(), err)
		}
	}

	if err := chartutil.ProcessDependencies(legacyChart, &releaseValues); err != nil {
		return nil, fmt.Errorf("error processing chart %q dependencies: %w", legacyChart.Name(), err)
	}

	if legacyChart.Metadata.Deprecated {
		log.Default.Warn(ctx, `Chart "%s:%s" is deprecated`, legacyChart.Name(), legacyChart.Metadata.Version)
	}

	caps, err := actionConfig.GetCapabilities()
	if err != nil {
		return nil, fmt.Errorf("error getting capabilities for chart %q: %w", legacyChart.Name(), err)
	}

	var isUpgrade bool
	switch deployType {
	case common.DeployTypeUpgrade, common.DeployTypeRollback:
		isUpgrade = true
	case common.DeployTypeInitial, common.DeployTypeInstall:
		isUpgrade = false
	}

	log.Default.Debug(ctx, "Rendering values for chart at %q", chartPath)
	values, err := chartutil.ToRenderValues(legacyChart, releaseValues, chartutil.ReleaseOptions{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Revision:  revision,
		IsInstall: !isUpgrade,
		IsUpgrade: isUpgrade,
	}, caps)
	if err != nil {
		return nil, fmt.Errorf("error building values for chart %q: %w", legacyChart.Name(), err)
	}

	finalValues := values.AsMap()
	hasClusterAccess := opts.Mapper != nil

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)
	legacyHookResources, generalManifestsBuf, notes, err := actionConfig.RenderResources(legacyChart, values, "", "", true, false, false, nil, hasClusterAccess, false)
	if err != nil {
		return nil, fmt.Errorf("error rendering resources for chart %q: %w", legacyChart.Name(), err)
	}

	var standaloneCRDs []*resrc.StandaloneCRD
	for _, crd := range legacyChart.CRDObjects() {
		for _, manifest := range releaseutil.SplitManifests(string(crd.File.Data)) {
			if res, err := resrc.NewStandaloneCRDFromManifest(manifest, resrc.StandaloneCRDFromManifestOptions{
				FilePath:         crd.Filename,
				DefaultNamespace: releaseNamespace,
				Mapper:           opts.Mapper,
			}); err != nil {
				return nil, fmt.Errorf("error constructing standalone CRD for chart at %q: %w", chartPath, err)
			} else {
				standaloneCRDs = append(standaloneCRDs, res)
			}
		}
	}

	var hookResources []*resrc.HookResource
	for _, hook := range legacyHookResources {
		for _, manifest := range releaseutil.SplitManifests(hook.Manifest) {
			if res, err := resrc.NewHookResourceFromManifest(manifest, resrc.HookResourceFromManifestOptions{
				DefaultNamespace: releaseNamespace,
				Mapper:           opts.Mapper,
				DiscoveryClient:  opts.DiscoveryClient,
			}); err != nil {
				return nil, fmt.Errorf("error constructing hook resource for chart at %q: %w", chartPath, err)
			} else {
				hookResources = append(hookResources, res)
			}
		}
	}

	var generalResources []*resrc.GeneralResource
	for _, manifest := range releaseutil.SplitManifests(generalManifestsBuf.String()) {
		if res, err := resrc.NewGeneralResourceFromManifest(manifest, resrc.GeneralResourceFromManifestOptions{
			DefaultNamespace: releaseNamespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing general resource for chart at %q: %w", chartPath, err)
		} else {
			generalResources = append(generalResources, res)
		}
	}

	return &ChartTree{
		standaloneCRDs:   standaloneCRDs,
		hookResources:    hookResources,
		generalResources: generalResources,
		notes:            notes,
		releaseValues:    releaseValues,
		finalValues:      finalValues,
		legacyChart:      legacyChart,
	}, nil
}

type ChartTreeOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
	StringSetValues []string
	SetValues       []string
	FileValues      []string
	ValuesFiles     []string
}

type ChartTree struct {
	standaloneCRDs   []*resrc.StandaloneCRD
	hookResources    []*resrc.HookResource
	generalResources []*resrc.GeneralResource
	notes            string
	releaseValues    map[string]interface{}
	finalValues      map[string]interface{}
	legacyChart      *chart.Chart
}

func (t *ChartTree) Name() string {
	return t.legacyChart.Name()
}

func (t *ChartTree) Path() string {
	return t.legacyChart.ChartFullPath()
}

func (t *ChartTree) StandaloneCRDs() []*resrc.StandaloneCRD {
	return t.standaloneCRDs
}

func (t *ChartTree) HookResources() []*resrc.HookResource {
	return t.hookResources
}

func (t *ChartTree) GeneralResources() []*resrc.GeneralResource {
	return t.generalResources
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

func (t *ChartTree) LegacyChart() *chart.Chart {
	return t.legacyChart
}
