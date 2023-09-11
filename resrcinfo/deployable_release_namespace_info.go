package resrcinfo

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/meta"
)

func NewDeployableReleaseNamespaceInfo(ctx context.Context, res *resrc.ReleaseNamespace, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableReleaseNamespaceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID)
	if getErr != nil {
		if isNotFoundErr(getErr) {
			return &DeployableReleaseNamespaceInfo{
				ResourceID: res.ResourceID,
				resource:   res,
			}, nil
		} else {
			return nil, fmt.Errorf("error getting release namespace: %w", getErr)
		}
	}
	getResource := resrc.NewRemoteResource(getObj, resrc.RemoteResourceOptions{
		FallbackNamespace: res.Namespace(),
		Mapper:            mapper,
	})

	dryApplyObject, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kubeclnt.KubeClientApplyOptions{
		DryRun: true,
	})
	var dryApplyResource *resrc.RemoteResource
	if dryApplyObject != nil {
		dryApplyResource = resrc.NewRemoteResource(dryApplyObject, resrc.RemoteResourceOptions{
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

	return &DeployableReleaseNamespaceInfo{
		ResourceID:       res.ResourceID,
		resource:         res,
		getResource:      getResource,
		dryApplyResource: dryApplyResource,
		exists:           getResource != nil,
		upToDate:         upToDateStatus,
	}, nil
}

type DeployableReleaseNamespaceInfo struct {
	*resrcid.ResourceID
	resource *resrc.ReleaseNamespace

	getResource      *resrc.RemoteResource
	dryApplyResource *resrc.RemoteResource

	exists   bool
	upToDate UpToDateStatus
}

func (i *DeployableReleaseNamespaceInfo) Resource() *resrc.ReleaseNamespace {
	return i.resource
}

func (i *DeployableReleaseNamespaceInfo) LiveResource() *resrc.RemoteResource {
	return i.getResource
}

func (i *DeployableReleaseNamespaceInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableReleaseNamespaceInfo) ShouldApply() bool {
	return i.exists && i.upToDate != UpToDateStatusYes
}

func (i *DeployableReleaseNamespaceInfo) ShouldKeepOnDelete() bool {
	return i.exists && i.getResource.KeepOnDelete()
}

func (i *DeployableReleaseNamespaceInfo) ShouldRepairManagedFields() bool {
	return i.exists && i.getResource.ManagedFieldsBroken() && i.ShouldApply()
}
