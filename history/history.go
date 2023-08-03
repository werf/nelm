package history

import (
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/chart"
	legacyRelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	drvr "helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/werf/plan"
	"helm.sh/helm/v3/pkg/werf/release"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

func NewHistory(releaseName, releaseNamespace string, driver driver.Driver, opts NewHistoryOptions) (*History, error) {
	legacyRels, err := driver.Query(map[string]string{"name": releaseName, "owner": "helm"})
	if err != nil && err != drvr.ErrReleaseNotFound {
		return nil, fmt.Errorf("error querying releases: %w", err)
	}
	releaseutil.SortByRevision(legacyRels)

	return &History{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
		legacyReleases:   legacyRels,
		driver:           driver,
		mapper:           opts.Mapper,
		discoveryClient:  opts.DiscoveryClient,
	}, nil
}

type NewHistoryOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type History struct {
	releaseName      string
	releaseNamespace string
	legacyReleases   []*legacyRelease.Release
	driver           driver.Driver
	mapper           meta.ResettableRESTMapper
	discoveryClient  discovery.CachedDiscoveryInterface
}

// Get last successfully deployed release since last attempt to uninstall release or from the beginning of history.
func (h *History) LastDeployedRelease() (*release.Release, error) {
	if h.Empty() {
		return nil, nil
	}

	var legacyRel *legacyRelease.Release
legacyRelLoop:
	for i := len(h.legacyReleases) - 1; i >= 0; i-- {
		switch h.legacyReleases[i].Info.Status {
		case legacyRelease.StatusDeployed, legacyRelease.StatusSuperseded:
			legacyRel = h.legacyReleases[i]
			break legacyRelLoop
		case legacyRelease.StatusUninstalled, legacyRelease.StatusUninstalling:
			break legacyRelLoop
		case legacyRelease.StatusUnknown, legacyRelease.StatusFailed, legacyRelease.StatusPendingInstall, legacyRelease.StatusPendingUpgrade, legacyRelease.StatusPendingRollback:
		}
	}

	if legacyRel == nil {
		return nil, nil
	}

	rel, err := release.BuildReleaseFromLegacyRelease(legacyRel, release.BuildReleaseFromLegacyReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	}).Load()
	if err != nil {
		return nil, fmt.Errorf("error loading release: %w", err)
	}

	return rel, nil
}

func (h *History) LastRelease() (*release.Release, error) {
	if h.Empty() {
		return nil, nil
	}

	legacyRel := h.legacyReleases[len(h.legacyReleases)-1]

	rel, err := release.BuildReleaseFromLegacyRelease(legacyRel, release.BuildReleaseFromLegacyReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	}).Load()
	if err != nil {
		return nil, fmt.Errorf("error loading release: %w", err)
	}

	return rel, nil
}

func (h *History) LastReleaseIsDeployed() (bool, error) {
	if h.Empty() {
		return false, nil
	}

	lastRel, err := h.LastRelease()
	if err != nil {
		return false, fmt.Errorf("error getting last release: %w", err)
	}

	lastDeployedRel, err := h.LastDeployedRelease()
	if err != nil {
		return false, fmt.Errorf("error getting last deployed release: %w", err)
	}

	return lastDeployedRel != nil && lastRel.Revision() == lastDeployedRel.Revision(), nil
}

func (h *History) DeployTypeForNextRelease() (plan.DeployType, error) {
	if h.Empty() {
		return plan.DeployTypeInitial, nil
	}

	lastDeployedRelease, err := h.LastDeployedRelease()
	if err != nil {
		return "", fmt.Errorf("error getting last deployed release: %w", err)
	}
	if lastDeployedRelease != nil {
		return plan.DeployTypeUpgrade, nil
	}

	return plan.DeployTypeInstall, nil
}

func (h *History) Empty() bool {
	return len(h.legacyReleases) == 0
}

func (h *History) NextReleaseRevision() (int, error) {
	if h.Empty() {
		return 1, nil
	}

	lastRelease, err := h.LastRelease()
	if err != nil {
		return 0, fmt.Errorf("error getting last release: %w", err)
	}

	return lastRelease.Revision() + 1, nil
}

func (h *History) BuildNextRelease(values map[string]interface{}, legacyChart *chart.Chart, standaloneCRDsManifests, hookManifests, generalManifests []string, notes string) (*release.Release, error) {
	revision, err := h.NextReleaseRevision()
	if err != nil {
		return nil, fmt.Errorf("error getting next release revision: %w", err)
	}

	rel, err := release.NewRelease(h.releaseName, h.releaseNamespace, revision, values, legacyChart, standaloneCRDsManifests, hookManifests, generalManifests, notes, release.NewReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	}).Load()
	if err != nil {
		return nil, fmt.Errorf("error loading release: %w", err)
	}

	deployType, err := h.DeployTypeForNextRelease()
	if err != nil {
		return nil, fmt.Errorf("error getting deploy type for next release: %w", err)
	}

	lastRelease, err := h.LastRelease()
	if err != nil {
		return nil, fmt.Errorf("error getting last release: %w", err)
	}

	switch deployType {
	case plan.DeployTypeInitial:
		// BACKCOMPAT: initial deploy attempt doesn't necessarily mean that the release was actually
		// successfully deployed and installed, but vanilla Helm marked it as such and set
		// firstDeployed and lastDeployed right away.
		now := time.Now()
		rel.SetFirstDeployed(now)
		rel.SetLastDeployed(now)
		rel.SetStatus(legacyRelease.StatusPendingInstall)
	case plan.DeployTypeInstall:
		rel.SetFirstDeployed(lastRelease.FirstDeployed())
		rel.SetLastDeployed(time.Now())
		rel.SetStatus(legacyRelease.StatusPendingInstall)
	case plan.DeployTypeUpgrade:
		rel.SetFirstDeployed(lastRelease.FirstDeployed())
		rel.SetLastDeployed(time.Now())
		rel.SetStatus(legacyRelease.StatusPendingUpgrade)
	case plan.DeployTypeRollback:
		rel.SetFirstDeployed(lastRelease.FirstDeployed())
		rel.SetLastDeployed(time.Now())
		rel.SetStatus(legacyRelease.StatusPendingRollback)
	}

	return rel, nil
}
