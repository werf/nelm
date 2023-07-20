package history

import (
	"fmt"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/werf/plan"
)

func NewHistory(releaseName string, driver driver.Driver) (*History, error) {
	releases, err := driver.Query(map[string]string{"name": releaseName, "owner": "helm"})
	if err != nil {
		return nil, fmt.Errorf("error querying releases: %w", err)
	}
	releaseutil.SortByRevision(releases)

	return &History{
		driver:   driver,
		releases: releases,
	}, nil
}

type History struct {
	driver   driver.Driver
	releases []*release.Release
}

// Get release that is a last attempt to install/upgrade/rollback since last attempt to uninstall release or from the beginning of history.
func (h *History) LastTriedRelease() *release.Release {
	if h.EmptyHistory() {
		return nil
	}

	for i := len(h.releases) - 1; i >= 0; i-- {
		switch h.releases[i].Info.Status {
		case release.StatusUninstalled, release.StatusUninstalling:
			return nil
		case release.StatusFailed, release.StatusPendingInstall, release.StatusPendingUpgrade, release.StatusPendingRollback, release.StatusDeployed, release.StatusSuperseded:
			return h.releases[i]
		case release.StatusUnknown:
		}
	}

	return nil
}

// Get last successfully deployed release since last attempt to uninstall release or from the beginning of history.
func (h *History) LastDeployedRelease() *release.Release {
	if h.EmptyHistory() {
		return nil
	}

	for i := len(h.releases) - 1; i >= 0; i-- {
		switch h.releases[i].Info.Status {
		case release.StatusDeployed, release.StatusSuperseded:
			return h.releases[i]
		case release.StatusUninstalled, release.StatusUninstalling:
			return nil
		case release.StatusUnknown, release.StatusFailed, release.StatusPendingInstall, release.StatusPendingUpgrade, release.StatusPendingRollback:
		}
	}

	return nil
}

func (h *History) FirstRelease() *release.Release {
	if h.EmptyHistory() {
		return nil
	}

	return h.releases[0]
}

func (h *History) LastRelease() *release.Release {
	if h.EmptyHistory() {
		return nil
	}

	return h.releases[len(h.releases)-1]
}

func (h *History) LastReleaseIsDeployed() bool {
	if h.EmptyHistory() {
		return false
	}

	lastRel := h.LastRelease()
	lastDeployedRel := h.LastDeployedRelease()

	return lastDeployedRel != nil && lastRel.Version == lastDeployedRel.Version
}

func (h *History) EmptyHistory() bool {
	return len(h.releases) == 0
}

func (h *History) DeployTypeForNextRelease() plan.DeployType {
	if h.EmptyHistory() {
		return plan.DeployTypeInitial
	}

	if h.LastDeployedRelease() != nil {
		return plan.DeployTypeUpgrade
	}

	return plan.DeployTypeInstall
}

func (h *History) RevisionForNextRelease() int {
	if h.EmptyHistory() {
		return 1
	}

	return h.LastRelease().Version + 1
}
