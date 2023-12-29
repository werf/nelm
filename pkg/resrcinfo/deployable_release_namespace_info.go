package resrcinfo

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"helm.sh/helm/v3/pkg/werf/utls"
	"k8s.io/apimachinery/pkg/api/meta"
)

func NewDeployableReleaseNamespaceInfo(ctx context.Context, res *resrc.ReleaseNamespace, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableReleaseNamespaceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kubeclnt.KubeClientGetOptions{
		TryCache: true,
	})
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
		FallbackNamespace: res.Name(),
		Mapper:            mapper,
	})

	if err := fixManagedFields(ctx, res.Name(), getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObject, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kubeclnt.KubeClientApplyOptions{
		DryRun: true,
	})
	var dryApplyResource *resrc.RemoteResource
	if dryApplyObject != nil {
		dryApplyResource = resrc.NewRemoteResource(dryApplyObject, resrc.RemoteResourceOptions{
			FallbackNamespace: res.Name(),
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

func (i *DeployableReleaseNamespaceInfo) DryApplyResource() *resrc.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableReleaseNamespaceInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableReleaseNamespaceInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == UpToDateStatusNo
}

func (i *DeployableReleaseNamespaceInfo) ShouldApply() bool {
	return i.exists && i.upToDate == UpToDateStatusUnknown
}

func (i *DeployableReleaseNamespaceInfo) ShouldKeepOnDelete() bool {
	return i.exists && i.getResource.KeepOnDelete()
}
