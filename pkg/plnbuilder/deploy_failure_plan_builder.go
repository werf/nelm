package plnbuilder

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/opertn"
	"github.com/werf/nelm/pkg/pln"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcinfo"
	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlshistor"
)

func NewDeployFailurePlanBuilder(
	deployPlan *pln.Plan,
	taskStore *statestore.TaskStore,
	hookResourcesInfos []*resrcinfo.DeployableHookResourceInfo,
	generalResourceInfos []*resrcinfo.DeployableGeneralResourceInfo,
	newRelease *rls.Release,
	history rlshistor.Historier,
	kubeClient kubeclnt.KubeClienter,
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	opts DeployFailurePlanBuilderOptions,
) *DeployFailurePlanBuilder {
	plan := pln.NewPlan()

	return &DeployFailurePlanBuilder{
		taskStore:            taskStore,
		hookResourceInfos:    hookResourcesInfos,
		generalResourceInfos: generalResourceInfos,
		newRelease:           newRelease,
		prevRelease:          opts.PrevRelease,
		history:              history,
		kubeClient:           kubeClient,
		dynamicClient:        dynamicClient,
		mapper:               mapper,
		deployPlan:           deployPlan,
		plan:                 plan,
		deletionTimeout:      opts.DeletionTimeout,
	}
}

type DeployFailurePlanBuilderOptions struct {
	PrevRelease     *rls.Release
	DeletionTimeout time.Duration
}

type DeployFailurePlanBuilder struct {
	taskStore            *statestore.TaskStore
	hookResourceInfos    []*resrcinfo.DeployableHookResourceInfo
	generalResourceInfos []*resrcinfo.DeployableGeneralResourceInfo
	newRelease           *rls.Release
	prevRelease          *rls.Release
	history              rlshistor.Historier
	kubeClient           kubeclnt.KubeClienter
	dynamicClient        dynamic.Interface
	mapper               meta.ResettableRESTMapper
	deployPlan           *pln.Plan
	plan                 *pln.Plan
	deletionTimeout      time.Duration
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

		trackReadinessOpID := fmt.Sprintf(opertn.TypeTrackResourceReadinessOperation + "/" + info.ID())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != opertn.StatusFailed {
			continue
		}

		cleanupOp := opertn.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
		)
		b.plan.AddOperation(cleanupOp)

		taskState := util.NewConcurrent(
			statestore.NewAbsenceTaskState(
				info.Name(),
				info.Namespace(),
				info.GroupVersionKind(),
				statestore.AbsenceTaskStateOptions{},
			),
		)
		b.taskStore.AddAbsenceTaskState(taskState)

		trackDeletionOp := opertn.NewTrackResourceAbsenceOperation(
			info.ResourceID,
			taskState,
			b.dynamicClient,
			b.mapper,
			opertn.TrackResourceAbsenceOperationOptions{
				Timeout: b.deletionTimeout,
			},
		)
		b.plan.AddOperation(trackDeletionOp)
		if err := b.plan.AddDependency(cleanupOp.ID(), trackDeletionOp.ID()); err != nil {
			return nil, fmt.Errorf("error adding dependency: %w", err)
		}
	}

	// TODO(ilya-lesikov): same as with hooks, refactor
	for _, info := range b.generalResourceInfos {
		if !info.ShouldCleanupOnFailed(prevReleaseFailed) || resrc.IsCRDFromGK(info.Resource().GroupVersionKind().GroupKind()) {
			continue
		}

		trackReadinessOpID := fmt.Sprintf(opertn.TypeTrackResourceReadinessOperation + "/" + info.ID())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != opertn.StatusFailed {
			continue
		}

		cleanupOp := opertn.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
		)
		b.plan.AddOperation(cleanupOp)

		taskState := util.NewConcurrent(
			statestore.NewAbsenceTaskState(
				info.Name(),
				info.Namespace(),
				info.GroupVersionKind(),
				statestore.AbsenceTaskStateOptions{},
			),
		)
		b.taskStore.AddAbsenceTaskState(taskState)

		trackDeletionOp := opertn.NewTrackResourceAbsenceOperation(
			info.ResourceID,
			taskState,
			b.dynamicClient,
			b.mapper,
			opertn.TrackResourceAbsenceOperationOptions{
				Timeout: b.deletionTimeout,
			},
		)
		b.plan.AddOperation(trackDeletionOp)
		if err := b.plan.AddDependency(cleanupOp.ID(), trackDeletionOp.ID()); err != nil {
			return nil, fmt.Errorf("error adding dependency: %w", err)
		}
	}

	return b.plan, nil
}
