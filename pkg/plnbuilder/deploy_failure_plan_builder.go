package plnbuilder

import (
	"context"
	"fmt"

	"nelm.sh/nelm/pkg/kubeclnt"
	"nelm.sh/nelm/pkg/opertn"
	"nelm.sh/nelm/pkg/pln"
	"nelm.sh/nelm/pkg/resrc"
	"nelm.sh/nelm/pkg/resrcinfo"
	"nelm.sh/nelm/pkg/rls"
	"nelm.sh/nelm/pkg/rlshistor"
)

func NewDeployFailurePlanBuilder(
	deployPlan *pln.Plan,
	hookResourcesInfos []*resrcinfo.DeployableHookResourceInfo,
	generalResourceInfos []*resrcinfo.DeployableGeneralResourceInfo,
	newRelease *rls.Release,
	history rlshistor.Historier,
	kubeClient kubeclnt.KubeClienter,
	opts DeployFailurePlanBuilderOptions,
) *DeployFailurePlanBuilder {
	plan := pln.NewPlan()

	return &DeployFailurePlanBuilder{
		hookResourceInfos:    hookResourcesInfos,
		generalResourceInfos: generalResourceInfos,
		newRelease:           newRelease,
		prevRelease:          opts.PrevRelease,
		history:              history,
		kubeClient:           kubeClient,
		deployPlan:           deployPlan,
		plan:                 plan,
	}
}

type DeployFailurePlanBuilderOptions struct {
	PrevRelease *rls.Release
}

type DeployFailurePlanBuilder struct {
	hookResourceInfos    []*resrcinfo.DeployableHookResourceInfo
	generalResourceInfos []*resrcinfo.DeployableGeneralResourceInfo
	newRelease           *rls.Release
	prevRelease          *rls.Release
	history              rlshistor.Historier
	kubeClient           kubeclnt.KubeClienter
	deployPlan           *pln.Plan
	plan                 *pln.Plan
}

func (b *DeployFailurePlanBuilder) Build(ctx context.Context) (*pln.Plan, error) {
	opFailRelease := opertn.NewFailReleaseOperation(b.newRelease, b.history)
	b.plan.AddOperation(opFailRelease)

	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	for _, info := range b.hookResourceInfos {
		if !info.ShouldCleanupOnFailed(prevReleaseFailed) || resrc.IsCRDFromGK(info.Resource().GroupVersionKind().GroupKind()) {
			continue
		}

		var hookPhase string
		if info.Resource().OnPreAnything() {
			hookPhase = "pre"
		} else {
			hookPhase = "post"
		}

		trackReadinessOpID := fmt.Sprintf("%s/%s-hook-resources/weight:%d", opertn.TypeTrackResourcesReadinessOperation, hookPhase, info.Resource().Weight())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != opertn.StatusFailed {
			continue
		}

		cleanupOp := opertn.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
		)
		b.plan.AddOperation(cleanupOp)
	}

	// TODO(ilya-lesikov): same as with hooks, refactor
	for _, info := range b.generalResourceInfos {
		if !info.ShouldCleanupOnFailed(prevReleaseFailed) || resrc.IsCRDFromGK(info.Resource().GroupVersionKind().GroupKind()) {
			continue
		}

		trackReadinessOpID := fmt.Sprintf("%s/general-resources/weight:%d", opertn.TypeTrackResourcesReadinessOperation, info.Resource().Weight())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != opertn.StatusFailed {
			continue
		}

		cleanupOp := opertn.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
		)
		b.plan.AddOperation(cleanupOp)
	}

	return b.plan, nil
}
