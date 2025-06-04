package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan/operation"
	info "github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/util"
)

func NewDeployFailurePlanBuilder(
	releaseName string,
	releaseNamespace string,
	deployType common.DeployType,
	deployPlan *Plan,
	taskStore *statestore.TaskStore,
	hookResourcesInfos []*info.DeployableHookResourceInfo,
	generalResourceInfos []*info.DeployableGeneralResourceInfo,
	history release.Historier,
	kubeClient kube.KubeClienter,
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	opts DeployFailurePlanBuilderOptions,
) *DeployFailurePlanBuilder {
	plan := NewPlan()

	return &DeployFailurePlanBuilder{
		releaseName:          releaseName,
		releaseNamespace:     releaseNamespace,
		deployType:           deployType,
		taskStore:            taskStore,
		hookResourceInfos:    hookResourcesInfos,
		generalResourceInfos: generalResourceInfos,
		newRelease:           opts.NewRelease,
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
	NewRelease      *release.Release
	PrevRelease     *release.Release
	DeletionTimeout time.Duration
}

type DeployFailurePlanBuilder struct {
	releaseName          string
	releaseNamespace     string
	deployType           common.DeployType
	taskStore            *statestore.TaskStore
	hookResourceInfos    []*info.DeployableHookResourceInfo
	generalResourceInfos []*info.DeployableGeneralResourceInfo
	newRelease           *release.Release
	prevRelease          *release.Release
	history              release.Historier
	kubeClient           kube.KubeClienter
	dynamicClient        dynamic.Interface
	mapper               meta.ResettableRESTMapper
	deployPlan           *Plan
	plan                 *Plan
	deletionTimeout      time.Duration
}

func (b *DeployFailurePlanBuilder) Build(ctx context.Context) (*Plan, error) {
	if b.newRelease != nil {
		opFailRelease := operation.NewFailReleaseOperation(b.newRelease, b.history)
		b.plan.AddOperation(opFailRelease)
	}

	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	hookInfos := lo.UniqBy(b.hookResourceInfos, func(info *info.DeployableHookResourceInfo) string {
		res := info.Resource()

		var pre, post bool
		switch b.deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			pre = res.OnPreInstall()
			post = res.OnPostInstall()
		case common.DeployTypeUpgrade:
			pre = res.OnPreUpgrade()
			post = res.OnPostUpgrade()
		case common.DeployTypeRollback:
			pre = res.OnPreRollback()
			post = res.OnPostRollback()
		case common.DeployTypeUninstall:
			pre = res.OnPreDelete()
			post = res.OnPostDelete()
		}

		return fmt.Sprintf("%s::%t::%t", info.ID(), pre, post)
	})

	for _, info := range hookInfos {
		if !info.ShouldCleanupOnFailed(prevReleaseFailed, b.releaseName, b.releaseNamespace) || util.IsCRDFromGK(info.Resource().GroupVersionKind().GroupKind()) {
			continue
		}

		trackReadinessOpID := fmt.Sprintf(operation.TypeTrackResourceReadinessOperation + "/" + info.ID())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != operation.StatusFailed {
			continue
		}

		cleanupOp := operation.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
			operation.DeleteResourceOperationOptions{},
		)
		b.plan.AddOperation(cleanupOp)

		taskState := kdutil.NewConcurrent(
			statestore.NewAbsenceTaskState(
				info.Name(),
				info.Namespace(),
				info.GroupVersionKind(),
				statestore.AbsenceTaskStateOptions{},
			),
		)
		b.taskStore.AddAbsenceTaskState(taskState)

		trackDeletionOp := operation.NewTrackResourceAbsenceOperation(
			info.ResourceID,
			taskState,
			b.dynamicClient,
			b.mapper,
			operation.TrackResourceAbsenceOperationOptions{
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
		if !info.ShouldCleanupOnFailed(prevReleaseFailed, b.releaseName, b.releaseNamespace) || util.IsCRDFromGK(info.Resource().GroupVersionKind().GroupKind()) {
			continue
		}

		trackReadinessOpID := fmt.Sprintf(operation.TypeTrackResourceReadinessOperation + "/" + info.ID())

		op, found := b.deployPlan.Operation(trackReadinessOpID)
		if !found || op.Status() != operation.StatusFailed {
			continue
		}

		cleanupOp := operation.NewDeleteResourceOperation(
			info.ResourceID,
			b.kubeClient,
			operation.DeleteResourceOperationOptions{},
		)
		b.plan.AddOperation(cleanupOp)

		taskState := kdutil.NewConcurrent(
			statestore.NewAbsenceTaskState(
				info.Name(),
				info.Namespace(),
				info.GroupVersionKind(),
				statestore.AbsenceTaskStateOptions{},
			),
		)
		b.taskStore.AddAbsenceTaskState(taskState)

		trackDeletionOp := operation.NewTrackResourceAbsenceOperation(
			info.ResourceID,
			taskState,
			b.dynamicClient,
			b.mapper,
			operation.TrackResourceAbsenceOperationOptions{
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
