package resrcinfo

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

func NewDeployableStandaloneCRDInfo(ctx context.Context, res *resrc.StandaloneCRD, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableStandaloneCRDInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID)
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
		FallbackNamespace: res.Namespace(),
		Mapper:            mapper,
	})

	dryApplyObj, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kubeclnt.KubeClientApplyOptions{
		DryRun: true,
	})
	var dryApplyResource *resrc.RemoteResource
	if dryApplyObj != nil {
		dryApplyResource = resrc.NewRemoteResource(dryApplyObj, resrc.RemoteResourceOptions{
			FallbackNamespace: res.Namespace(),
			Mapper:            mapper,
		})
	}

	var upToDateStatus UpToDateStatus
	if getResource == nil {
		upToDateStatus = UpToDateStatusNo
	} else if dryApplyResource == nil {
		upToDateStatus = UpToDateStatusUnknown
	} else {
		different := diffGetAndDryApplyObjects(getResource.Unstructured(), dryApplyResource.Unstructured())

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

func (i *DeployableStandaloneCRDInfo) ShouldRepairManagedFields() bool {
	return i.exists && i.getResource.ManagedFieldsBroken() && (i.ShouldUpdate() || i.ShouldApply())
}

func (i *DeployableStandaloneCRDInfo) LiveUID() (uid types.UID, found bool) {
	if !i.exists {
		return types.UID(0), false
	}

	return i.getResource.Unstructured().GetUID(), true
}
