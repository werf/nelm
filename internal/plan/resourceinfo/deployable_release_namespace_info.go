package resourceinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

func NewDeployableReleaseNamespaceInfo(ctx context.Context, res *resource.ReleaseNamespace, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*DeployableReleaseNamespaceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, res.ResourceID, kube.KubeClientGetOptions{
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
	getResource := resource.NewRemoteResource(getObj, resource.RemoteResourceOptions{
		FallbackNamespace: res.Name(),
		Mapper:            mapper,
	})

	if err := fixManagedFieldsInCluster(ctx, res.Name(), getObj, getResource, kubeClient, mapper); err != nil {
		return nil, fmt.Errorf("error fixing managed fields for resource %q: %w", res.HumanID(), err)
	}

	dryApplyObject, _ := kubeClient.Apply(ctx, res.ResourceID, res.Unstructured(), kube.KubeClientApplyOptions{
		DryRun: true,
	})
	var dryApplyResource *resource.RemoteResource
	if dryApplyObject != nil {
		dryApplyResource = resource.NewRemoteResource(dryApplyObject, resource.RemoteResourceOptions{
			FallbackNamespace: res.Name(),
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
	*id.ResourceID
	resource *resource.ReleaseNamespace

	getResource      *resource.RemoteResource
	dryApplyResource *resource.RemoteResource

	exists   bool
	upToDate resource.UpToDateStatus
}

func (i *DeployableReleaseNamespaceInfo) Resource() *resource.ReleaseNamespace {
	return i.resource
}

func (i *DeployableReleaseNamespaceInfo) LiveResource() *resource.RemoteResource {
	return i.getResource
}

func (i *DeployableReleaseNamespaceInfo) DryApplyResource() *resource.RemoteResource {
	return i.dryApplyResource
}

func (i *DeployableReleaseNamespaceInfo) ShouldCreate() bool {
	return !i.exists
}

func (i *DeployableReleaseNamespaceInfo) ShouldUpdate() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusNo
}

func (i *DeployableReleaseNamespaceInfo) ShouldApply() bool {
	return i.exists && i.upToDate == resource.UpToDateStatusUnknown
}

func (i *DeployableReleaseNamespaceInfo) ShouldKeepOnDelete(releaseName, releaseNamespace string) bool {
	return i.exists && i.getResource.KeepOnDelete(releaseName, releaseNamespace)
}
