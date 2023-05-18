package release

import (
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
	legacyRelease "helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/plan"
	"helm.sh/helm/v3/pkg/werf/resource"
	"sigs.k8s.io/yaml"
)

func NewDeployReleaseBuilder(
	deployType plan.DeployType,
	releaseName string,
	releaseNamespace string,
	chart *chart.Chart,
	valuesMap map[string]interface{},
	revision int,
) *DeployReleaseBuilder {
	return &DeployReleaseBuilder{
		deployType:       deployType,
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
		chart:            chart,
		valuesMap:        valuesMap,
		revision:         revision,
	}
}

type DeployReleaseBuilder struct {
	deployType       plan.DeployType
	releaseName      string
	releaseNamespace string
	chart            *chart.Chart
	valuesMap        map[string]interface{}
	revision         int
	helmResources    []*resource.HelmResource
	helmHooks        []*resource.HelmHook
	notes            string

	prevRelease *legacyRelease.Release
}

func (b *DeployReleaseBuilder) WithPrevRelease(release *legacyRelease.Release) *DeployReleaseBuilder {
	b.prevRelease = release
	return b
}

func (b *DeployReleaseBuilder) WithHelmResources(resources []*resource.HelmResource) *DeployReleaseBuilder {
	b.helmResources = resources
	return b
}

func (b *DeployReleaseBuilder) WithHelmHooks(hooks []*resource.HelmHook) *DeployReleaseBuilder {
	b.helmHooks = hooks
	return b
}

func (b *DeployReleaseBuilder) WithNotes(notes string) *DeployReleaseBuilder {
	b.notes = notes
	return b
}

func (b *DeployReleaseBuilder) BuildPendingRelease() (*legacyRelease.Release, error) {
	var (
		firstDeployed, lastDeployed helmtime.Time
		status                      legacyRelease.Status
	)

	switch b.deployType {
	case plan.DeployTypeInitial:
		// BACKCOMPAT: initial deploy attempt doesn't necessarily mean that the release was actually
		// successfully deployed and installed, but vanilla Helm marked it as such and set
		// firstDeployed and lastDeployed right away.
		now := helmtime.Now()
		firstDeployed = now
		lastDeployed = now
		status = legacyRelease.StatusPendingInstall
	case plan.DeployTypeInstall:
		firstDeployed = b.prevRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = legacyRelease.StatusPendingInstall
	case plan.DeployTypeUpgrade:
		firstDeployed = b.prevRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = legacyRelease.StatusPendingUpgrade
	case plan.DeployTypeRollback:
		firstDeployed = b.prevRelease.Info.FirstDeployed
		lastDeployed = helmtime.Now()
		status = legacyRelease.StatusPendingRollback
	}

	// FIXME(ilya-lesikov): additional sorting?
	var helmResourceManifests string
	for _, res := range b.helmResources {
		marshalledResByte, err := yaml.Marshal(res.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error marshalling resource %s to YAML: %s", res, err)
		}

		if helmResourceManifests == "" {
			helmResourceManifests = string(marshalledResByte)
		} else {
			helmResourceManifests = helmResourceManifests + "---\n" + string(marshalledResByte)
		}
	}

	// FIXME(ilya-lesikov): additional sorting?
	var legacyHelmHooks []*legacyRelease.Hook
	for _, hook := range b.helmHooks {
		marshalledHookByte, err := yaml.Marshal(hook.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error marshalling hook %s to YAML: %s", hook, err)
		}

		var legacyHookEvents []legacyRelease.HookEvent
		for _, hookType := range hook.Types() {
			if hookType == common.HelmHookTypeTestLegacy {
				hookType = common.HelmHookTypeTest
			}

			legacyHookEvents = append(legacyHookEvents, legacyRelease.HookEvent(hookType))
		}

		var legacyHookDeletePolicies []legacyRelease.HookDeletePolicy
		for _, hookDeletePolicy := range hook.DeletePolicies() {
			legacyHookDeletePolicies = append(legacyHookDeletePolicies, legacyRelease.HookDeletePolicy(hookDeletePolicy))
		}

		legacyHook := &legacyRelease.Hook{
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

	pendingRelease := &legacyRelease.Release{
		Name:      b.releaseName,
		Namespace: b.releaseNamespace,
		Chart:     b.chart,
		Config:    b.valuesMap,
		Info: &legacyRelease.Info{
			Status:        status,
			FirstDeployed: firstDeployed,
			LastDeployed:  lastDeployed,
			Notes:         b.notes,
		},
		Version:  b.revision,
		Manifest: helmResourceManifests,
		Hooks:    legacyHelmHooks,
	}

	return pendingRelease, nil
}

func (b *DeployReleaseBuilder) PromotePendingReleaseToSucceeded(pendingRel *legacyRelease.Release) *legacyRelease.Release {
	pendingRel.Info.Status = legacyRelease.StatusDeployed
	return pendingRel
}

func (b *DeployReleaseBuilder) PromotePreviousReleaseToSuperseded(prevRel *legacyRelease.Release) *legacyRelease.Release {
	prevRel.Info.Status = legacyRelease.StatusSuperseded
	return prevRel
}

func (b *DeployReleaseBuilder) PromotePendingReleaseToFailed(pendingRel *legacyRelease.Release) *legacyRelease.Release {
	pendingRel.Info.Status = legacyRelease.StatusFailed
	return pendingRel
}
