package release

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource"
)

func NewRelease(name, namespace string, revision int, overrideValues map[string]interface{}, legacyChart *chart.Chart, hookResources []*resource.HookResource, generalResources []*resource.GeneralResource, notes string, opts ReleaseOptions) (*Release, error) {
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	sort.SliceStable(hookResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(hookResources[i].ResourceID, hookResources[j].ResourceID)
	})

	sort.SliceStable(generalResources, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(generalResources[i].ResourceID, generalResources[j].ResourceID)
	})

	var status helmrelease.Status
	if opts.Status == "" {
		status = helmrelease.StatusUnknown
	} else {
		status = opts.Status
	}

	notes = strings.TrimRightFunc(notes, unicode.IsSpace)

	if opts.InfoAnnotations == nil {
		opts.InfoAnnotations = map[string]string{}
	}

	if opts.Labels == nil {
		opts.Labels = map[string]string{}
	}

	return &Release{
		appVersion:       legacyChart.Metadata.AppVersion,
		chartName:        legacyChart.Metadata.Name,
		chartVersion:     legacyChart.Metadata.Version,
		firstDeployed:    opts.FirstDeployed,
		generalResources: generalResources,
		hookResources:    hookResources,
		infoAnnotations:  opts.InfoAnnotations,
		labels:           opts.Labels,
		lastDeployed:     opts.LastDeployed,
		legacyChart:      legacyChart,
		mapper:           opts.Mapper,
		name:             name,
		namespace:        namespace,
		notes:            notes,
		revision:         revision,
		status:           status,
		overrideValues:   overrideValues,
	}, nil
}

type ReleaseOptions struct {
	FirstDeployed   time.Time
	InfoAnnotations map[string]string
	Labels          map[string]string
	LastDeployed    time.Time
	Mapper          meta.ResettableRESTMapper
	Status          helmrelease.Status
}

func NewReleaseFromLegacyRelease(legacyRelease *helmrelease.Release, opts ReleaseFromLegacyReleaseOptions) (*Release, error) {
	var hookResources []*resource.HookResource
	for _, legacyHook := range legacyRelease.Hooks {
		if res, err := resource.NewHookResourceFromManifest(legacyHook.Manifest, resource.HookResourceFromManifestOptions{
			FilePath:         legacyHook.Path,
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing hook resource from manifest for legacy release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			hookResources = append(hookResources, res)
		}
	}

	var generalResources []*resource.GeneralResource
	for _, manifest := range releaseutil.SplitManifests(legacyRelease.Manifest) {
		if res, err := resource.NewGeneralResourceFromManifest(manifest, resource.GeneralResourceFromManifestOptions{
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing general resource from manifest for legacy release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			generalResources = append(generalResources, res)
		}
	}

	rel, err := NewRelease(legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, legacyRelease.Config, legacyRelease.Chart, hookResources, generalResources, legacyRelease.Info.Notes, ReleaseOptions{
		FirstDeployed:   legacyRelease.Info.FirstDeployed.Time,
		InfoAnnotations: legacyRelease.Info.Annotations,
		Labels:          legacyRelease.Labels,
		LastDeployed:    legacyRelease.Info.LastDeployed.Time,
		Mapper:          opts.Mapper,
		Status:          legacyRelease.Info.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("error building release %q (namespace: %q, revision: %d): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
	}

	return rel, nil
}

type ReleaseFromLegacyReleaseOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type Release struct {
	appVersion       string
	chartName        string
	chartVersion     string
	firstDeployed    time.Time
	generalResources []*resource.GeneralResource
	hookResources    []*resource.HookResource
	infoAnnotations  map[string]string
	labels           map[string]string
	lastDeployed     time.Time
	legacyChart      *chart.Chart
	mapper           meta.ResettableRESTMapper
	name             string
	namespace        string
	notes            string
	revision         int
	status           helmrelease.Status
	overrideValues   map[string]interface{}
}

func (r *Release) Name() string {
	return r.name
}

func (r *Release) Namespace() string {
	return r.namespace
}

func (r *Release) Revision() int {
	return r.revision
}

func (r *Release) OverrideValues() map[string]interface{} {
	return r.overrideValues
}

func (r *Release) LegacyChart() *chart.Chart {
	return r.legacyChart
}

func (r *Release) HookResources() []*resource.HookResource {
	return r.hookResources
}

func (r *Release) GeneralResources() []*resource.GeneralResource {
	return r.generalResources
}

func (r *Release) Notes() string {
	return r.notes
}

func (r *Release) Status() helmrelease.Status {
	return r.status
}

func (r *Release) FirstDeployed() time.Time {
	return r.firstDeployed
}

func (r *Release) LastDeployed() time.Time {
	return r.lastDeployed
}

func (r *Release) AppVersion() string {
	return r.appVersion
}

func (r *Release) ChartName() string {
	return r.chartName
}

func (r *Release) ChartVersion() string {
	return r.chartVersion
}

func (r *Release) InfoAnnotations() map[string]string {
	return r.infoAnnotations
}

func (r *Release) Labels() map[string]string {
	return r.labels
}

func (r *Release) ID() string {
	return fmt.Sprintf("%s:%s:%d", r.namespace, r.name, r.revision)
}

func (r *Release) HumanID() string {
	return fmt.Sprintf("%s:%s/%d", r.namespace, r.name, r.revision)
}

func (r *Release) Fail() {
	r.status = helmrelease.StatusFailed
}

func (r *Release) Supersede() {
	r.status = helmrelease.StatusSuperseded
}

func (r *Release) Succeed() {
	r.status = helmrelease.StatusDeployed
}

func (r *Release) Succeeded() bool {
	switch r.status {
	case helmrelease.StatusDeployed,
		helmrelease.StatusSuperseded,
		helmrelease.StatusUninstalled:
		return true
	}

	return false
}

func (r *Release) Failed() bool {
	switch r.status {
	case helmrelease.StatusFailed,
		helmrelease.StatusUnknown,
		helmrelease.StatusPendingInstall,
		helmrelease.StatusPendingUpgrade,
		helmrelease.StatusPendingRollback,
		helmrelease.StatusUninstalling:
		return true
	}

	return false
}

func (r *Release) Pend(deployType common.DeployType) {
	r.status = helmrelease.StatusPendingInstall

	switch deployType {
	case common.DeployTypeInitial,
		common.DeployTypeInstall:
		r.status = helmrelease.StatusPendingInstall
	case common.DeployTypeUpgrade:
		r.status = helmrelease.StatusPendingUpgrade
	case common.DeployTypeRollback:
		r.status = helmrelease.StatusPendingRollback
	}

	now := time.Now()
	if r.firstDeployed.IsZero() {
		r.firstDeployed = now
	}
	r.lastDeployed = now
}

func (r *Release) Skip() {
	r.status = helmrelease.StatusSkipped
}
