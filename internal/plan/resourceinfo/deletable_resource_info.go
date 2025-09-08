package resourceinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

func BuildDeletableResourceInfo(ctx context.Context, localRes *resource.DeletableResource, releaseName, releaseNamespace string, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*DeletableResourceInfo, error) {
	noDeleteInfo := &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
	}

	if localRes.KeepOnDelete || localRes.Ownership == common.OwnershipEveryone {
		return noDeleteInfo, nil
	}

	getObj, getErr := kubeClient.Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			return noDeleteInfo, nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	getMeta := id.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)

	if err := resource.ValidateResourcePolicy(getMeta); err != nil {
		return noDeleteInfo, nil
	} else {
		if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
			return noDeleteInfo, nil
		}
	}

	if resource.Orphaned(getMeta, releaseName, releaseNamespace) {
		return noDeleteInfo, nil
	}

	return &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
		GetResult:     getObj,
		MustDelete:    true,
		// TODO: make switchable
		MustTrackAbsence: true,
	}, nil
}

type DeletableResourceInfo struct {
	*id.ResourceMeta

	LocalResource *resource.DeletableResource
	GetResult     *unstructured.Unstructured

	MustDelete       bool
	MustTrackAbsence bool
}
