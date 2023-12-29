package rls

import (
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"nelm.sh/nelm/pkg/common"
	"nelm.sh/nelm/pkg/resrc"
)

func NewRelease(name, namespace string, revision int, values map[string]interface{}, legacyChart *chart.Chart, hookResources []*resrc.HookResource, generalResources []*resrc.GeneralResource, notes string, opts ReleaseOptions) (*Release, error) {
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	var status release.Status
	if opts.Status == "" {
		status = release.StatusUnknown
	} else {
		status = opts.Status
	}

	return &Release{
		name:             name,
		namespace:        namespace,
		revision:         revision,
		values:           values,
		legacyChart:      legacyChart,
		mapper:           opts.Mapper,
		status:           status,
		hookResources:    hookResources,
		generalResources: generalResources,
		notes:            notes,
		firstDeployed:    opts.FirstDeployed,
		lastDeployed:     opts.LastDeployed,
	}, nil
}

type ReleaseOptions struct {
	Status        release.Status
	FirstDeployed time.Time
	LastDeployed  time.Time
	Mapper        meta.ResettableRESTMapper
}

func NewReleaseFromLegacyRelease(legacyRelease *release.Release, opts ReleaseFromLegacyReleaseOptions) (*Release, error) {
	var hookResources []*resrc.HookResource
	for _, legacyHook := range legacyRelease.Hooks {
		if res, err := resrc.NewHookResourceFromManifest(legacyHook.Manifest, resrc.HookResourceFromManifestOptions{
			FilePath:         legacyHook.Path,
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing hook resource from manifest for legacy release %q (namespace: %q, revision: %q): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			hookResources = append(hookResources, res)
		}
	}

	var generalResources []*resrc.GeneralResource
	for _, manifest := range releaseutil.SplitManifests(legacyRelease.Manifest) {
		if res, err := resrc.NewGeneralResourceFromManifest(manifest, resrc.GeneralResourceFromManifestOptions{
			DefaultNamespace: legacyRelease.Namespace,
			Mapper:           opts.Mapper,
			DiscoveryClient:  opts.DiscoveryClient,
		}); err != nil {
			return nil, fmt.Errorf("error constructing general resource from manifest for legacy release %q (namespace: %q, revision: %q): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
		} else {
			generalResources = append(generalResources, res)
		}
	}

	rel, err := NewRelease(legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, legacyRelease.Config, legacyRelease.Chart, hookResources, generalResources, legacyRelease.Info.Notes, ReleaseOptions{
		Status:        legacyRelease.Info.Status,
		FirstDeployed: legacyRelease.Info.FirstDeployed.Time,
		LastDeployed:  legacyRelease.Info.LastDeployed.Time,
		Mapper:        opts.Mapper,
	})
	if err != nil {
		return nil, fmt.Errorf("error building release %q (namespace: %q, revision: %q): %w", legacyRelease.Name, legacyRelease.Namespace, legacyRelease.Version, err)
	}

	return rel, nil
}

type ReleaseFromLegacyReleaseOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type Release struct {
	name        string
	namespace   string
	revision    int
	values      map[string]interface{}
	legacyChart *chart.Chart
	mapper      meta.ResettableRESTMapper

	status        release.Status
	firstDeployed time.Time
	lastDeployed  time.Time

	hookResources    []*resrc.HookResource
	generalResources []*resrc.GeneralResource
	notes            string
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

func (r *Release) Values() map[string]interface{} {
	return r.values
}

func (r *Release) LegacyChart() *chart.Chart {
	return r.legacyChart
}

func (r *Release) HookResources() []*resrc.HookResource {
	return r.hookResources
}

func (r *Release) GeneralResources() []*resrc.GeneralResource {
	return r.generalResources
}

func (r *Release) Notes() string {
	return r.notes
}

func (r *Release) Status() release.Status {
	return r.status
}

func (r *Release) FirstDeployed() time.Time {
	return r.firstDeployed
}

func (r *Release) LastDeployed() time.Time {
	return r.lastDeployed
}

func (r *Release) ID() string {
	return fmt.Sprintf("%s:%s:%d", r.namespace, r.name, r.revision)
}

func (r *Release) HumanID() string {
	return fmt.Sprintf("%s:%s/%d", r.namespace, r.name, r.revision)
}

func (r *Release) Fail() {
	r.status = release.StatusFailed
}

func (r *Release) Supersede() {
	r.status = release.StatusSuperseded
}

func (r *Release) Succeed() {
	r.status = release.StatusDeployed
}

func (r *Release) Succeeded() bool {
	switch r.status {
	case release.StatusDeployed,
		release.StatusSuperseded,
		release.StatusUninstalled:
		return true
	}

	return false
}

func (r *Release) Failed() bool {
	switch r.status {
	case release.StatusFailed,
		release.StatusUnknown,
		release.StatusPendingInstall,
		release.StatusPendingUpgrade,
		release.StatusPendingRollback,
		release.StatusUninstalling:
		return true
	}

	return false
}

func (r *Release) Pend(deployType common.DeployType) {
	r.status = release.StatusPendingInstall

	switch deployType {
	case common.DeployTypeInitial,
		common.DeployTypeInstall:
		r.status = release.StatusPendingInstall
	case common.DeployTypeUpgrade:
		r.status = release.StatusPendingUpgrade
	case common.DeployTypeRollback:
		r.status = release.StatusPendingRollback
	}

	now := time.Now()
	if r.firstDeployed.IsZero() {
		r.firstDeployed = now
	}
	r.lastDeployed = now
}
