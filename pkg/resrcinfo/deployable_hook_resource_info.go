package resrcinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
	"github.com/werf/nelm/pkg/utls"
)

func NewDeployableHookResourceInfo(ctx context.Context, res *resrc.HookResource, releaseNamespace string, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableHookResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kubeclnt.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if isNotFoundErr(getErr) || isNoSuchKindErr(getErr) {
			return &DeployableHookResourceInfo{
				ResourceID: res.ResourceID,
				resource:   res,
			}, nil
		} else {
			return nil, fmt.Errorf("error getting hook resource: %w", getErr)
		}
	}
	getResource := resrc.NewRemoteResource(getObj, resrc.RemoteResourceOptions{
		FallbackNamespace: releaseNamespace,
		Mapper:            mapper,
	})

	if err := fixManagedFields(ctx, releaseNamespace, getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObj, dryApplyErr := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kubeclnt.KubeClientApplyOptions{
		DryRun: true,
	})
	if dryApplyErr != nil && isImmutableErr(dryApplyErr) && !res.Recreate() {
		return nil, fmt.Errorf("error dry applying hook resource: %w", dryApplyErr)
	}
	var dryApplyResource *resrc.RemoteResource
	if dryApplyObj != nil {
		dryApplyResource = resrc.NewRemoteResource(dryApplyObj, resrc.RemoteResourceOptions{
			FallbackNamespace: releaseNamespace,
			Mapper:            mapper,
		})
	}

	var upToDateStatus UpToDateStatus
	if getResource == nil {
		upToDateStatus = UpToDateStatusNo
	} else if dryApplyResource == nil {
		if isImmutableErr(dryApplyErr) {
			upToDateStatus = UpToDateStatusNo
		} else {
			upToDateStatus = UpToDateStatusUnknown
		}
	} else {
		different, err := utls.ResourcesReallyDiffer(getResource.Unstructured(), dryApplyResource.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error diffing live and dry-apply versions of resource %q: %w", res.HumanID(), err)
		}

		if different {
			upToDateStatus = UpToDateStatusNo
		} else {
			upToDateStatus = UpToDateStatusYes
		}
	}

	return &DeployableHookResourceInfo{
		ResourceID:       res.ResourceID,
		resource:         res,
		getResource:      getResource,
		dryApplyResource: dryApplyResource,
		dryApplyErr:      dryApplyErr,
		exists:           getResource != nil,
		upToDate:         upToDateStatus,
	}, nil
}

type DeployableHookResourceInfo struct {
	*resrcid.ResourceID
	resource *resrc.HookResource

	getResource      *resrc.RemoteResource
	dryApplyResource *resrc.RemoteResource
	dryApplyErr      error

	exists   bool
	upToDate UpToDateStatus
}

func (i *DeployableHookResourceInfo) Resource() *resrc.HookResource {
	return i.resource
}

func (i *DeployableHookResourceInfo) LiveResource() *resrc.RemoteResource {
	return i.getResource
}

func (i *DeployableHookResourceInfo) DryApplyResource() *resrc.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableHookResourceInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableHookResourceInfo) ShouldRecreate() bool {
	return i.exists && i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == UpToDateStatusNo && !i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldApply() bool {
	return i.exists && i.upToDate == UpToDateStatusUnknown && !i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldCleanup() bool {
	return (i.exists || i.shouldDeploy()) && i.resource.DeleteOnSucceeded() && !i.ShouldKeepOnDelete()
}

func (i *DeployableHookResourceInfo) ShouldCleanupOnFailed(prevRelFailed bool) bool {
	return i.ShouldTrackReadiness(prevRelFailed) && i.resource.DeleteOnFailed() && !i.ShouldKeepOnDelete()
}

func (i *DeployableHookResourceInfo) ShouldKeepOnDelete() bool {
	return i.resource.KeepOnDelete() || (i.exists && i.getResource.KeepOnDelete())
}

func (i *DeployableHookResourceInfo) ShouldTrackReadiness(prevRelFailed bool) bool {
	if resrc.IsCRDFromGK(i.resource.GroupVersionKind().GroupKind()) {
		return false
	}

	if i.shouldDeploy() {
		return true
	} else if prevRelFailed && i.exists {
		return true
	}

	return false
}

func (i *DeployableHookResourceInfo) ForceReplicas() (replicas int, set bool) {
	if !i.ShouldCreate() && !i.ShouldRecreate() {
		return 0, false
	}

	return i.resource.DefaultReplicasOnCreation()
}

func (i *DeployableHookResourceInfo) LiveUID() (uid types.UID, found bool) {
	if !i.exists {
		return types.UID(0), false
	}

	return i.getResource.Unstructured().GetUID(), true
}

func (i *DeployableHookResourceInfo) shouldDeploy() bool {
	return i.ShouldCreate() || i.ShouldRecreate() || i.ShouldUpdate() || i.ShouldApply()
}
