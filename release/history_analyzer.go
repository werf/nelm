package release

import (
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/werf/plan"
)

func NewHistoryAnalyzer(releases []*release.Release) *HistoryAnalyzer {
	releaseutil.SortByRevision(releases)

	return &HistoryAnalyzer{
		releases: releases,
	}
}

type HistoryAnalyzer struct {
	releases []*release.Release
}

// Get release that is a last attempt to install/upgrade/rollback since last attempt to uninstall release or from the beginning of history.
func (a *HistoryAnalyzer) LastTriedRelease() *release.Release {
	if a.EmptyHistory() {
		return nil
	}

	for i := len(a.releases) - 1; i >= 0; i-- {
		switch a.releases[i].Info.Status {
		case release.StatusUninstalled, release.StatusUninstalling:
			return nil
		case release.StatusFailed, release.StatusPendingInstall, release.StatusPendingUpgrade, release.StatusPendingRollback, release.StatusDeployed, release.StatusSuperseded:
			return a.releases[i]
		case release.StatusUnknown:
		}
	}

	return nil
}

// Get last succesfully deployed release since last attempt to uninstall release or from the beginning of history.
func (a *HistoryAnalyzer) LastDeployedRelease() *release.Release {
	if a.EmptyHistory() {
		return nil
	}

	for i := len(a.releases) - 1; i >= 0; i-- {
		switch a.releases[i].Info.Status {
		case release.StatusDeployed, release.StatusSuperseded:
			return a.releases[i]
		case release.StatusUninstalled, release.StatusUninstalling:
			return nil
		case release.StatusUnknown, release.StatusFailed, release.StatusPendingInstall, release.StatusPendingUpgrade, release.StatusPendingRollback:
		}
	}

	return nil
}

// Get very last release regardless of its status.
func (a *HistoryAnalyzer) LastRelease() *release.Release {
	if a.EmptyHistory() {
		return nil
	}

	return a.releases[len(a.releases)-1]
}

func (a *HistoryAnalyzer) LastReleaseDeployed() bool {
	if a.EmptyHistory() {
		return false
	}

	lastRel := a.LastRelease()
	lastDeployedRel := a.LastDeployedRelease()

	return lastDeployedRel != nil && lastRel.Version == lastDeployedRel.Version
}

func (a *HistoryAnalyzer) FirstRelease() *release.Release {
	if a.EmptyHistory() {
		return nil
	}

	return a.releases[0]
}

func (a *HistoryAnalyzer) EmptyHistory() bool {
	return len(a.releases) == 0
}

func (a *HistoryAnalyzer) DeployTypeForNewRelease() plan.DeployType {
	if a.EmptyHistory() {
		return plan.DeployTypeInitial
	}

	if a.LastDeployedRelease() != nil {
		return plan.DeployTypeUpgrade
	}

	return plan.DeployTypeInstall
}

func (a *HistoryAnalyzer) RevisionForNewRelease() int {
	if a.EmptyHistory() {
		return 1
	}

	return a.LastRelease().Version + 1
}
