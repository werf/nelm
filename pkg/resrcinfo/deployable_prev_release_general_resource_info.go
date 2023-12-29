package resrcinfo

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"nelm.sh/nelm/pkg/kubeclnt"
	"nelm.sh/nelm/pkg/resrc"
	"nelm.sh/nelm/pkg/resrcid"
)

func NewDeployablePrevReleaseGeneralResourceInfo(ctx context.Context, res *resrc.GeneralResource, releaseNamespace string, kubeClient kubeclnt.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployablePrevReleaseGeneralResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kubeclnt.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if isNotFoundErr(getErr) || isNoSuchKindErr(getErr) {
			return &DeployablePrevReleaseGeneralResourceInfo{
				ResourceID: res.ResourceID,
				resource:   res,
			}, nil
		} else {
			return nil, fmt.Errorf("error getting previous release general resource: %w", getErr)
		}
	}
	getResource := resrc.NewRemoteResource(getObj, resrc.RemoteResourceOptions{
		FallbackNamespace: releaseNamespace,
		Mapper:            mapper,
	})

	return &DeployablePrevReleaseGeneralResourceInfo{
		ResourceID:  res.ResourceID,
		resource:    res,
		getResource: getResource,
		exists:      getResource != nil,
	}, nil
}

type DeployablePrevReleaseGeneralResourceInfo struct {
	*resrcid.ResourceID
	resource *resrc.GeneralResource

	getResource *resrc.RemoteResource

	exists bool
}

func (i *DeployablePrevReleaseGeneralResourceInfo) Resource() *resrc.GeneralResource {
	return i.resource
}

func (i *DeployablePrevReleaseGeneralResourceInfo) LiveResource() *resrc.RemoteResource {
	return i.getResource
}

func (i *DeployablePrevReleaseGeneralResourceInfo) ShouldKeepOnDelete() bool {
	return i.resource.KeepOnDelete() || (i.exists && i.getResource.KeepOnDelete())
}

func (i *DeployablePrevReleaseGeneralResourceInfo) ShouldDelete(curReleaseExistingResourcesUIDs []types.UID) bool {
	if !i.exists {
		return false
	}

	if i.ShouldKeepOnDelete() {
		return false
	}

	return !lo.Contains(curReleaseExistingResourcesUIDs, i.getResource.Unstructured().GetUID())
}
