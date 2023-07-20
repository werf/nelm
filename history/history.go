package history

import (
	"fmt"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	drvr "helm.sh/helm/v3/pkg/storage/driver"
	helmtime "helm.sh/helm/v3/pkg/time"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/plan"
	"helm.sh/helm/v3/pkg/werf/resource"
)

func NewHistory(releaseName, releaseNamespace string, driver driver.Driver) (*History, error) {
	releases, err := driver.Query(map[string]string{"name": releaseName, "owner": "helm"})
	if err != nil && err != drvr.ErrReleaseNotFound {
		return nil, fmt.Errorf("error querying releases: %w", err)
	}
	releaseutil.SortByRevision(releases)

	return &History{
		driver:           driver,
		releases:         releases,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}, nil
}

type History struct {
	driver           driver.Driver
	releases         []*release.Release
	releaseName      string
	releaseNamespace string
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

func (h *History) BuildNextRelease(helmHooks []*resource.HelmHook, helmResources []*resource.HelmResource, notes string, chart *chart.Chart, values map[string]interface{}) (*release.Release, error) {
	var (
		firstDeployed, lastDeployed helmtime.Time
		status                      release.Status
	)

	deployType := h.DeployTypeForNextRelease()
	lastRelease := h.LastRelease()

	switch deployType {
	case plan.DeployTypeInitial:
		// BACKCOMPAT: initial deploy attempt doesn't necessarily mean that the release was actually
		// successfully deployed and installed, but vanilla Helm marked it as such and set
		// firstDeployed and lastDeployed right away.
		now := helmtime.Now()
		firstDeployed = now
		lastDeployed = now
		status = release.StatusPendingInstall
	case plan.DeployTypeInstall:
		firstDeployed = lastRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = release.StatusPendingInstall
	case plan.DeployTypeUpgrade:
		firstDeployed = lastRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = release.StatusPendingUpgrade
	case plan.DeployTypeRollback:
		firstDeployed = lastRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = release.StatusPendingRollback
	}

	// FIXME(ilya-lesikov): additional sorting?
	var helmResourceManifests string
	for _, res := range helmResources {
		marshalledResByte, err := yaml.Marshal(res.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error marshalling resource %q to YAML: %w", res.String(), err)
		}

		if helmResourceManifests == "" {
			helmResourceManifests = string(marshalledResByte)
		} else {
			helmResourceManifests = helmResourceManifests + "---\n" + string(marshalledResByte)
		}
	}

	// FIXME(ilya-lesikov): additional sorting?
	var legacyHelmHooks []*release.Hook
	for _, hook := range helmHooks {
		marshalledHookByte, err := yaml.Marshal(hook.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error marshalling hook %q to YAML: %w", hook, err)
		}

		var legacyHookEvents []release.HookEvent
		for _, hookType := range hook.Types() {
			if hookType == common.HelmHookTypeTestLegacy {
				hookType = common.HelmHookTypeTest
			}

			legacyHookEvents = append(legacyHookEvents, release.HookEvent(hookType))
		}

		var legacyHookDeletePolicies []release.HookDeletePolicy
		for _, hookDeletePolicy := range hook.DeletePolicies() {
			legacyHookDeletePolicies = append(legacyHookDeletePolicies, release.HookDeletePolicy(hookDeletePolicy))
		}

		legacyHook := &release.Hook{
			Name:           hook.Name(),
			Kind:           hook.GroupVersionKind().Kind,
			Path:           hook.FilePath(),
			Manifest:       string(marshalledHookByte),
			Events:         legacyHookEvents,
			Weight:         hook.Weight(),
			DeletePolicies: legacyHookDeletePolicies,
		}

		legacyHelmHooks = append(legacyHelmHooks, legacyHook)
	}

	pendingRelease := &release.Release{
		Name:      h.releaseName,
		Namespace: h.releaseNamespace,
		Chart:     chart,
		Config:    values,
		Info: &release.Info{
			Status:        status,
			FirstDeployed: firstDeployed,
			LastDeployed:  lastDeployed,
			Notes:         notes,
		},
		Version:  h.RevisionForNextRelease(),
		Manifest: helmResourceManifests,
		Hooks:    legacyHelmHooks,
	}

	return pendingRelease, nil
}

func (h *History) PromoteReleaseToSucceeded(rel *release.Release) *release.Release {
	rel.Info.Status = release.StatusDeployed
	return rel
}

func (h *History) PromoteReleaseToSuperseded(rel *release.Release) *release.Release {
	rel.Info.Status = release.StatusSuperseded
	return rel
}

func (h *History) PromoteReleaseToFailed(rel *release.Release) *release.Release {
	rel.Info.Status = release.StatusFailed
	return rel
}
