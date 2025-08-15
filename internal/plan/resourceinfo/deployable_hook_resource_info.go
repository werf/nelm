package resourceinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

func NewDeployableHookResourceInfo(ctx context.Context, res *resource.HookResource, releaseNamespace string, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableHookResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kube.KubeClientGetOptions{
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
	getResource := resource.NewRemoteResource(getObj, resource.RemoteResourceOptions{
		FallbackNamespace: releaseNamespace,
		Mapper:            mapper,
	})

	if err := fixManagedFieldsInCluster(ctx, releaseNamespace, getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObj, dryApplyErr := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kube.KubeClientApplyOptions{
		DryRun: true,
	})
	if dryApplyErr != nil && isImmutableErr(dryApplyErr) && !res.Recreate() {
		return nil, fmt.Errorf("error dry applying hook resource: %w", dryApplyErr)
	}
	var dryApplyResource *resource.RemoteResource
	if dryApplyObj != nil {
		dryApplyResource = resource.NewRemoteResource(dryApplyObj, resource.RemoteResourceOptions{
			FallbackNamespace: releaseNamespace,
			Mapper:            mapper,
		})
	}

	var upToDateStatus resource.UpToDateStatus
	if getResource == nil {
		upToDateStatus = resource.UpToDateStatusNo
	} else if dryApplyResource == nil {
		if isImmutableErr(dryApplyErr) {
			upToDateStatus = resource.UpToDateStatusNo
		} else {
			upToDateStatus = resource.UpToDateStatusUnknown
		}
	} else {
		different, err := util.ResourcesReallyDiffer(getResource.Unstructured(), dryApplyResource.Unstructured())
		if err != nil {
			return nil, fmt.Errorf("error diffing live and dry-apply versions of resource %q: %w", res.HumanID(), err)
		}

		if different {
			upToDateStatus = resource.UpToDateStatusNo
		} else {
			upToDateStatus = resource.UpToDateStatusYes
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
	*id.ResourceID
	resource *resource.HookResource

	getResource      *resource.RemoteResource
	dryApplyResource *resource.RemoteResource
	dryApplyErr      error

	exists   bool
	upToDate resource.UpToDateStatus
}

func (i *DeployableHookResourceInfo) Resource() *resource.HookResource {
	return i.resource
}

func (i *DeployableHookResourceInfo) LiveResource() *resource.RemoteResource {
	return i.getResource
}

func (i *DeployableHookResourceInfo) DryApplyResource() *resource.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableHookResourceInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableHookResourceInfo) ShouldRecreate() bool {
	return i.exists && i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusNo && !i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldApply() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusUnknown && !i.resource.Recreate()
}

func (i *DeployableHookResourceInfo) ShouldCleanup(releaseName, releaseNamespace string) bool {
	return (i.exists || i.shouldDeploy()) && i.resource.DeleteOnSucceeded() && !i.ShouldKeepOnDelete(releaseName, releaseNamespace)
}

func (i *DeployableHookResourceInfo) ShouldCleanupOnFailed(prevRelFailed bool, releaseName, releaseNamespace string) bool {
	return i.ShouldTrackReadiness(prevRelFailed) && i.resource.DeleteOnFailed() && !i.ShouldKeepOnDelete(releaseName, releaseNamespace)
}

func (i *DeployableHookResourceInfo) ShouldKeepOnDelete(releaseName, releaseNamespace string) bool {
	return i.resource.KeepOnDelete(releaseNamespace) || (i.exists && i.getResource.KeepOnDelete(releaseName, releaseNamespace))
}

func (i *DeployableHookResourceInfo) ShouldTrackReadiness(prevRelFailed bool) bool {
	if util.IsCRDFromGK(i.resource.GroupVersionKind().GroupKind()) ||
		i.Resource().TrackTerminationMode() == multitrack.NonBlocking {
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
