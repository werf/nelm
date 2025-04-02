package resourceinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

func NewDeployableStandaloneCRDInfo(ctx context.Context, res *resource.StandaloneCRD, releaseNamespace string, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableStandaloneCRDInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kube.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if isNotFoundErr(getErr) {
			return &DeployableStandaloneCRDInfo{
				ResourceID: res.ResourceID,
				resource:   res,
			}, nil
		} else {
			return nil, fmt.Errorf("error getting standalone CRD: %w", getErr)
		}
	}
	getResource := resource.NewRemoteResource(getObj, resource.RemoteResourceOptions{
		FallbackNamespace: releaseNamespace,
		Mapper:            mapper,
	})

	if err := fixManagedFieldsInCluster(ctx, releaseNamespace, getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObj, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kube.KubeClientApplyOptions{
		DryRun: true,
	})
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
		upToDateStatus = resource.UpToDateStatusUnknown
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

	return &DeployableStandaloneCRDInfo{
		ResourceID:       res.ResourceID,
		resource:         res,
		getResource:      getResource,
		dryApplyResource: dryApplyResource,
		exists:           getResource != nil,
		upToDate:         upToDateStatus,
	}, nil
}

type DeployableStandaloneCRDInfo struct {
	*id.ResourceID
	resource *resource.StandaloneCRD

	getResource      *resource.RemoteResource
	dryApplyResource *resource.RemoteResource

	exists   bool
	upToDate resource.UpToDateStatus
}

func (i *DeployableStandaloneCRDInfo) Resource() *resource.StandaloneCRD {
	return i.resource
}

func (i *DeployableStandaloneCRDInfo) LiveResource() *resource.RemoteResource {
	return i.getResource
}

func (i *DeployableStandaloneCRDInfo) DryApplyResource() *resource.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableStandaloneCRDInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableStandaloneCRDInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusNo
}

func (i *DeployableStandaloneCRDInfo) ShouldApply() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusUnknown
}

func (i *DeployableStandaloneCRDInfo) LiveUID() (uid types.UID, found bool) {
	if !i.exists {
		return types.UID(0), false
	}

	return i.getResource.Unstructured().GetUID(), true
}
