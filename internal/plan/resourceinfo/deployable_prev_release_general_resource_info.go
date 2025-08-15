package resourceinfo

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

func NewDeployablePrevReleaseGeneralResourceInfo(ctx context.Context, res *resource.GeneralResource, releaseNamespace string, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployablePrevReleaseGeneralResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kube.KubeClientGetOptions{
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
	getResource := resource.NewRemoteResource(getObj, resource.RemoteResourceOptions{
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
	*id.ResourceID
	resource *resource.GeneralResource

	getResource *resource.RemoteResource

	exists bool
}

func (i *DeployablePrevReleaseGeneralResourceInfo) Resource() *resource.GeneralResource {
	return i.resource
}

func (i *DeployablePrevReleaseGeneralResourceInfo) LiveResource() *resource.RemoteResource {
	return i.getResource
}

func (i *DeployablePrevReleaseGeneralResourceInfo) ShouldKeepOnDelete(releaseName, releaseNamespace string) bool {
	return i.resource.KeepOnDelete(releaseNamespace) || (i.exists && i.getResource.KeepOnDelete(releaseName, releaseNamespace))
}

func (i *DeployablePrevReleaseGeneralResourceInfo) ShouldDelete(curReleaseExistingResourcesUIDs []types.UID, releaseName, releaseNamespace string, deployType common.DeployType) bool {
	if !i.exists {
		return false
	}

	if i.ShouldKeepOnDelete(releaseName, releaseNamespace) {
		return false
	}

	if deployType == common.DeployTypeUninstall {
		return true
	}

	return !lo.Contains(curReleaseExistingResourcesUIDs, i.getResource.Unstructured().GetUID())
}
