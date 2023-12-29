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

func NewDeployableStandaloneCRDInfo(ctx context.Context, res *resrc.StandaloneCRD, releaseNamespace string, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableStandaloneCRDInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kubeclnt.KubeClientGetOptions{
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
	getResource := resrc.NewRemoteResource(getObj, resrc.RemoteResourceOptions{
		FallbackNamespace: releaseNamespace,
		Mapper:            mapper,
	})

	if err := fixManagedFields(ctx, releaseNamespace, getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObj, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kubeclnt.KubeClientApplyOptions{
		DryRun: true,
	})
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
		upToDateStatus = UpToDateStatusUnknown
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
	*resrcid.ResourceID
	resource *resrc.StandaloneCRD

	getResource      *resrc.RemoteResource
	dryApplyResource *resrc.RemoteResource

	exists   bool
	upToDate UpToDateStatus
}

func (i *DeployableStandaloneCRDInfo) Resource() *resrc.StandaloneCRD {
	return i.resource
}

func (i *DeployableStandaloneCRDInfo) LiveResource() *resrc.RemoteResource {
	return i.getResource
}

func (i *DeployableStandaloneCRDInfo) DryApplyResource() *resrc.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableStandaloneCRDInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableStandaloneCRDInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == UpToDateStatusNo
}

func (i *DeployableStandaloneCRDInfo) ShouldApply() bool {
	return i.exists && i.upToDate == UpToDateStatusUnknown
}

func (i *DeployableStandaloneCRDInfo) LiveUID() (uid types.UID, found bool) {
	if !i.exists {
		return types.UID(0), false
	}

	return i.getResource.Unstructured().GetUID(), true
}
