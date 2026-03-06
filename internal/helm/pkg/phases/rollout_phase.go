package phases

import (
	"bytes"
	"fmt"

	"github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/phases/stages"
	rel "github.com/werf/3p-helm/pkg/release"
)

func NewRolloutPhase(release *rel.Release, stagesSplitter Splitter, kubeClient kube.Interface) *RolloutPhase {
	return &RolloutPhase{
		Release:        release,
		stagesSplitter: stagesSplitter,
		kubeClient:     kubeClient,
	}
}

type RolloutPhase struct {
	SortedStages stages.SortedStageList
	Release      *rel.Release

	stagesSplitter Splitter
	kubeClient     kube.Interface
}

func (m *RolloutPhase) ParseStagesFromString(manifests string) (*RolloutPhase, error) {
	resources, err := m.kubeClient.Build(bytes.NewBufferString(manifests), false)
	if err != nil {
		return nil, fmt.Errorf("error building kubernetes objects from manifests: %w", err)
	}

	return m.ParseStages(resources)
}

func (m *RolloutPhase) ParseStages(resources kube.ResourceList) (*RolloutPhase, error) {
	var err error
	m.SortedStages, err = m.stagesSplitter.Split(resources)
	if err != nil {
		return nil, fmt.Errorf("error splitting rollout stage resources list: %w", err)
	}

	return m, nil
}

func (m *RolloutPhase) GenerateStagesExternalDeps(stagesExternalDepsGenerator ExternalDepsGenerator) error {
	if err := stagesExternalDepsGenerator.Generate(m.SortedStages); err != nil {
		return fmt.Errorf("error generating external deps for stages: %w", err)
	}

	if err := m.validateStagesExternalDeps(); err != nil {
		return fmt.Errorf("error validating external deps: %w", err)
	}

	return nil
}

func (m *RolloutPhase) DeployedResources() kube.ResourceList {
	lastDeployedStageIndex := m.LastDeployedStageIndex()
	if lastDeployedStageIndex == nil {
		return nil
	}

	return m.SortedStages.MergedDesiredResourcesInStagesRange(0, *lastDeployedStageIndex)
}

func (m *RolloutPhase) AllResources() kube.ResourceList {
	return m.SortedStages.MergedDesiredResources()
}

func (m *RolloutPhase) LastDeployedStageIndex() *int {
	if !m.IsPhaseStarted() {
		return nil
	}

	lastStage := len(m.SortedStages) - 1

	if m.IsPhaseCompleted() {
		return &lastStage
	}

	// Phase started but not completed.
	if m.Release.Info.LastStage == nil {
		return &lastStage
	} else {
		return m.Release.Info.LastStage
	}
}

func (m *RolloutPhase) IsPhaseStarted() bool {
	if m.Release.Info.LastPhase == nil {
		return true
	}

	switch *m.Release.Info.LastPhase {
	case rel.PhaseRollout, rel.PhaseUninstall, rel.PhaseHooksPost, rel.PhaseHooksPre:
		return true
	default:
		return false
	}
}

func (m *RolloutPhase) IsPhaseCompleted() bool {
	if m.Release.Info.LastPhase == nil {
		return true
	}

	switch *m.Release.Info.LastPhase {
	case rel.PhaseRollout:
		if m.Release.Info.LastStage == nil {
			return true
		} else {
			return *m.Release.Info.LastStage == len(m.SortedStages)-1
		}
	case rel.PhaseHooksPost:
		return true
	default:
		return false
	}
}

func (m *RolloutPhase) validateStagesExternalDeps() error {
	phaseDesiredResources := m.SortedStages.MergedDesiredResources()

	for _, stage := range m.SortedStages {
		for _, stageExtDep := range stage.ExternalDependencies {
			for _, phaseDesiredRes := range phaseDesiredResources {
				if kube.ResourceNameNamespaceKind(stageExtDep.Info) == kube.ResourceNameNamespaceKind(phaseDesiredRes) {
					return fmt.Errorf("resources from current release can't be external dependencies: remove external dependency on %q", kube.ResourceNameNamespaceKind(stageExtDep.Info))
				}
			}
		}
	}

	return nil
}
