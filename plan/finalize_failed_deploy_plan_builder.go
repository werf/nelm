package plan

import (
	legacyRelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/resource"
)

func NewFinalizeFailedDeployPlanBuilder(failedRel *legacyRelease.Release) *FinalizeFailedDeployPlanBuilder {
	return &FinalizeFailedDeployPlanBuilder{
		failedRelease: failedRel,
	}
}

type FinalizeFailedDeployPlanBuilder struct {
	failedRelease       *legacyRelease.Release
	referencesToCleanup []resource.Referencer
}

func (b *FinalizeFailedDeployPlanBuilder) WithReferencesToCleanup(refs []resource.Referencer) *FinalizeFailedDeployPlanBuilder {
	b.referencesToCleanup = refs
	return b
}

func (b *FinalizeFailedDeployPlanBuilder) Build() *Plan {
	var phaseFailedRelease *Phase
	failReleaseOp := NewOperationUpdateReleases().AddReleases(b.failedRelease)
	phaseFailedRelease = NewPhase(PhaseTypeFailRelease).AddOperations(failReleaseOp)

	var phaseCleanup *Phase
	if len(b.referencesToCleanup) > 0 {
		cleanupOp := NewOperationDelete().AddTargets(b.referencesToCleanup...)
		phaseCleanup = NewPhase(PhaseTypeCleanup).AddOperations(cleanupOp)
	}

	plan := NewPlan(PlanTypeFinalizeFailedDeploy)

	plan.AddPhases(phaseFailedRelease)
	if phaseCleanup != nil {
		plan.AddPhases(phaseCleanup)
	}

	return plan
}
