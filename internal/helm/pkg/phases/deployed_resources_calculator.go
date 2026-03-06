package phases

import (
	"fmt"
	"math"

	"github.com/werf/3p-helm/pkg/kube"
	rel "github.com/werf/3p-helm/pkg/release"
)

func NewDeployedResourcesCalculator(history []*rel.Release, stagesSplitter Splitter, kubeClient kube.Interface) *DeployedResourcesCalculator {
	return &DeployedResourcesCalculator{
		history:        history,
		stagesSplitter: stagesSplitter,
		kubeClient:     kubeClient,
	}
}

type DeployedResourcesCalculator struct {
	history        []*rel.Release
	stagesSplitter Splitter
	kubeClient     kube.Interface
}

func (c *DeployedResourcesCalculator) Calculate() (kube.ResourceList, error) {
	lastDeployedReleaseIndex := c.lastDeployedReleaseIndex()
	lastUninstalledReleaseIndex := c.lastUninstalledReleaseIndex()

	startAt := c.calculateRevisionToStartAt(lastDeployedReleaseIndex, lastUninstalledReleaseIndex)
	if startAt == nil {
		return nil, nil
	}

	result := kube.ResourceList{}
	for i := *startAt; i < len(c.history); i++ {
		release := c.history[i]

		switch release.Info.Status {
		case
			rel.StatusDeployed,
			rel.StatusSuperseded,
			rel.StatusFailed,
			rel.StatusPendingInstall,
			rel.StatusPendingUpgrade,
			rel.StatusPendingRollback,
			rel.StatusUninstalling:
			mainPhase, err := NewRolloutPhase(release, c.stagesSplitter, c.kubeClient).
				ParseStagesFromString(release.Manifest)
			if err != nil {
				return nil, fmt.Errorf("error creating main phase: %w", err)
			}

			result.Merge(mainPhase.DeployedResources())
		case rel.StatusUninstalled, rel.StatusUnknown:
		default:
			panic(fmt.Sprintf("unexpected release status: %s", release.Info.Status))
		}
	}

	return result, nil
}

func (c *DeployedResourcesCalculator) calculateRevisionToStartAt(lastDeployedReleaseIndex, lastUninstalledReleaseIndex *int) *int {
	if lastDeployedReleaseIndex == nil && lastUninstalledReleaseIndex == nil {
		firstRev := 0
		return &firstRev
	} else if lastDeployedReleaseIndex != nil && lastUninstalledReleaseIndex == nil {
		return lastDeployedReleaseIndex
	} else if lastDeployedReleaseIndex == nil && lastUninstalledReleaseIndex != nil {
		if *lastUninstalledReleaseIndex == (len(c.history) - 1) {
			return nil
		}

		result := *lastUninstalledReleaseIndex + 1
		return &result
	} else {
		if *lastUninstalledReleaseIndex == (len(c.history) - 1) {
			return lastDeployedReleaseIndex
		}

		result := int(math.Max(float64(*lastDeployedReleaseIndex), float64(*lastUninstalledReleaseIndex+1)))
		return &result
	}
}

func (c *DeployedResourcesCalculator) lastDeployedReleaseIndex() *int {
	for i := len(c.history) - 1; i >= 0; i-- {
		if c.history[i].Info.Status == rel.StatusDeployed || c.history[i].Info.Status == rel.StatusSuperseded {
			return &i
		}
	}

	return nil
}

func (c *DeployedResourcesCalculator) lastUninstalledReleaseIndex() *int {
	for i := len(c.history) - 1; i >= 0; i-- {
		if c.history[i].Info.Status == rel.StatusUninstalled {
			return &i
		}
	}

	return nil
}
