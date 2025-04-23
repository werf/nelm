package chart

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/yaml"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/cli/values"
	"github.com/werf/3p-helm/pkg/downloader"
	helmengine "github.com/werf/3p-helm/pkg/engine"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/internal/resource"
)

func NewChartTree(ctx context.Context, chartPath, releaseName, releaseNamespace string, revision int, deployType common.DeployType, opts ChartTreeOptions) (*ChartTree, error) {
	valOpts := &values.Options{
		StringValues: opts.StringSetValues,
		Values:       opts.SetValues,
		FileValues:   opts.FileValues,
		ValueFiles:   opts.ValuesFiles,
	}

	getters := getter.All(helm_v3.Settings)

	log.Default.Debug(ctx, "Merging values for chart tree at %q", chartPath)
	releaseValues, err := valOpts.MergeValues(getters)
	if err != nil {
		return nil, fmt.Errorf("error merging values for chart tree at %q: %w", chartPath, err)
	}

	log.Default.Debug(ctx, "Loading chart at %q", chartPath)
	legacyChart, err := loader.Load(chartPath)
	if err != nil {
		var e *downloader.ErrRepoNotFound
		if errors.As(err, &e) {
			return nil, fmt.Errorf("%w. Please add the missing repos via 'helm repo add'", e)
		}

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

	if err := chartutil.ProcessDependenciesWithMerge(legacyChart, &releaseValues); err != nil {
		return nil, fmt.Errorf("error processing chart %q dependencies: %w", legacyChart.Name(), err)
	}

	if legacyChart.Metadata.Deprecated {
		log.Default.Warn(ctx, `Chart "%s:%s" is deprecated`, legacyChart.Name(), legacyChart.Metadata.Version)
	}

	// TODO(ilya-lesikov): pass custom local api versions
	caps, err := BuildCapabilities(ctx, BuildCapabilitiesOptions{
		DiscoveryClient: opts.DiscoveryClient,
		KubeVersion:     opts.KubeVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("build capabilities for chart %q: %w", legacyChart.Name(), err)
	}

	if legacyChart.Metadata.KubeVersion != "" && !chartutil.IsCompatibleRange(legacyChart.Metadata.KubeVersion, caps.KubeVersion.String()) {
		return nil, fmt.Errorf("chart requires kubeVersion: %s which is incompatible with Kubernetes %s", legacyChart.Metadata.KubeVersion, caps.KubeVersion.String())
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

	var engine *helmengine.Engine
	if opts.KubeConfig != nil {
		engine = lo.ToPtr(helmengine.New(opts.KubeConfig.RestConfig))
	} else {
		engine = lo.ToPtr(helmengine.Engine{})
	}
	engine.EnableDNS = opts.AllowDNSRequests

	log.Default.Debug(ctx, "Rendering resources for chart at %q", chartPath)

	var standaloneCRDs []*resource.StandaloneCRD
	for _, crd := range legacyChart.CRDObjects() {
		for _, manifest := range releaseutil.SplitManifests(string(crd.File.Data)) {
			if res, err := resource.NewStandaloneCRDFromManifest(manifest, resource.StandaloneCRDFromManifestOptions{
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

	renderedTemplates, err := engine.Render(legacyChart, values)
	if err != nil {
		return nil, fmt.Errorf("render resources for chart %q: %w", legacyChart.Name(), err)
	}

	log.Default.TraceStruct(ctx, renderedTemplates, "Rendered contents of templates/:")

	var (
		hookResources    []*resource.HookResource
		generalResources []*resource.GeneralResource
	)
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

			if head.Metadata != nil && resource.IsHook(head.Metadata.Annotations) {
				res, err := resource.NewHookResourceFromManifest(manifest, resource.HookResourceFromManifestOptions{
					DefaultNamespace: releaseNamespace,
					Mapper:           opts.Mapper,
					DiscoveryClient:  opts.DiscoveryClient,
					FilePath:         filePath,
				})
				if err != nil {
					return nil, fmt.Errorf("error constructing hook resource for chart at %q: %w", chartPath, err)
				}

				hookResources = append(hookResources, res)
			} else {
				res, err := resource.NewGeneralResourceFromManifest(manifest, resource.GeneralResourceFromManifestOptions{
					DefaultNamespace: releaseNamespace,
					Mapper:           opts.Mapper,
					DiscoveryClient:  opts.DiscoveryClient,
					FilePath:         filePath,
				})
				if err != nil {
					return nil, fmt.Errorf("error constructing general resource for chart at %q: %w", chartPath, err)
				}

				generalResources = append(generalResources, res)
			}
		}
	}

	notes := BuildNotes(legacyChart.Name(), renderedTemplates, BuildNotesOptions{
		RenderSubchartNotes: opts.SubNotes,
	})

	sort.SliceStable(standaloneCRDs, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(standaloneCRDs[i].ResourceID, standaloneCRDs[j].ResourceID)
	})

	sort.SliceStable(hookResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(hookResources[i].ResourceID, hookResources[j].ResourceID)
	})

	sort.SliceStable(generalResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(generalResources[i].ResourceID, generalResources[j].ResourceID)
	})

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

// TODO(ilya-lesikov): pass missing options from top-level
type ChartTreeOptions struct {
	Mapper           meta.ResettableRESTMapper
	DiscoveryClient  discovery.CachedDiscoveryInterface
	KubeConfig       *kube.KubeConfig
	StringSetValues  []string
	SetValues        []string
	FileValues       []string
	ValuesFiles      []string
	SubNotes         bool
	KubeVersion      *chartutil.KubeVersion
	AllowDNSRequests bool
}

type ChartTree struct {
	standaloneCRDs   []*resource.StandaloneCRD
	hookResources    []*resource.HookResource
	generalResources []*resource.GeneralResource
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

func (t *ChartTree) StandaloneCRDs() []*resource.StandaloneCRD {
	return t.standaloneCRDs
}

func (t *ChartTree) HookResources() []*resource.HookResource {
	return t.hookResources
}

func (t *ChartTree) GeneralResources() []*resource.GeneralResource {
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
