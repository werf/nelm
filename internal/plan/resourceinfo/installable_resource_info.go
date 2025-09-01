package resourceinfo

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

type ResourceInstallType string

const (
	ResourceInstallTypeNone     ResourceInstallType = "none"
	ResourceInstallTypeCreate   ResourceInstallType = "create"
	ResourceInstallTypeRecreate ResourceInstallType = "recreate"
	ResourceInstallTypeUpdate   ResourceInstallType = "update"
	ResourceInstallTypeApply    ResourceInstallType = "apply"
)

// TODO(v2): keep annotation should probably forbid resource recreations
func BuildInstallableResourceInfo(ctx context.Context, localRes *resource.InstallableResource, releaseNamespace string, prevRelFailed bool, kubeClient kube.KubeClienter, mapper meta.ResettableRESTMapper) (*InstallableResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			return &InstallableResourceInfo{
				ResourceMeta:                 localRes.ResourceMeta,
				LocalResource:                localRes,
				MustInstall:                  ResourceInstallTypeCreate,
				MustDeleteOnSuccessfulDeploy: mustDeleteOnSuccessfulDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace),
				MustDeleteOnFailedDeploy:     mustDeleteOnFailedDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace),
				MustTrackReadiness:           mustTrackReadiness(localRes, ResourceInstallTypeCreate, false, prevRelFailed),
			}, nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	var err error
	getObj, err = fixManagedFieldsInCluster(ctx, releaseNamespace, getObj, localRes.ResourceMeta, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", localRes.IDHuman(), err)
	}

	dryApplyObj, dryApplyErr := kubeClient.Apply(ctx, localRes.ResourceSpec, kube.KubeClientApplyOptions{
		DryRun: true,
	})

	installType, err := resourceInstallType(localRes, getObj, dryApplyObj, dryApplyErr)
	if err != nil {
		return nil, fmt.Errorf("determine install type for resource %q: %w", localRes.IDHuman(), err)
	}

	getMeta := id.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)

	return &InstallableResourceInfo{
		ResourceMeta:                 localRes.ResourceMeta,
		LocalResource:                localRes,
		GetResult:                    getObj,
		DryApplyResult:               dryApplyObj,
		DryApplyErr:                  dryApplyErr,
		MustInstall:                  installType,
		MustDeleteOnSuccessfulDeploy: mustDeleteOnSuccessfulDeploy(localRes, getMeta, installType, releaseNamespace),
		MustDeleteOnFailedDeploy:     mustDeleteOnFailedDeploy(localRes, getMeta, installType, releaseNamespace),
		MustTrackReadiness:           mustTrackReadiness(localRes, installType, true, prevRelFailed),
	}, nil
}

type InstallableResourceInfo struct {
	*id.ResourceMeta

	LocalResource  *resource.InstallableResource
	GetResult      *unstructured.Unstructured
	DryApplyResult *unstructured.Unstructured
	DryApplyErr    error

	MustInstall                  ResourceInstallType
	MustDeleteOnSuccessfulDeploy bool
	MustDeleteOnFailedDeploy     bool
	MustTrackReadiness           bool
}

func resourceInstallType(localRes *resource.InstallableResource, getObj, dryApplyObj *unstructured.Unstructured, dryApplyErr error) (ResourceInstallType, error) {
	if dryApplyErr != nil && kube.IsImmutableErr(dryApplyErr) && !localRes.Recreate {
		return "", fmt.Errorf("immutable fields change in resource %q, but recreation is not requested: %w", localRes.IDHuman(), dryApplyErr)
	}

	if localRes.Recreate {
		return ResourceInstallTypeRecreate, nil
	}

	if dryApplyErr != nil {
		return ResourceInstallTypeApply, nil
	}

	if different, err := util.ResourcesReallyDiffer(getObj, dryApplyObj); err != nil {
		return "", fmt.Errorf("diff live and dry-apply versions of resource %q: %w", localRes.IDHuman(), err)
	} else if different {
		return ResourceInstallTypeUpdate, nil
	}

	return ResourceInstallTypeNone, nil
}

func mustDeleteOnSuccessfulDeploy(localRes *resource.InstallableResource, getMeta *id.ResourceMeta, installType ResourceInstallType, releaseNamespace string) bool {
	if !localRes.DeleteOnSucceeded || localRes.KeepOnDelete {
		return false
	}

	if getMeta != nil {
		if err := resource.ValidateResourcePolicy(getMeta); err != nil {
			return false
		} else {
			if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
				return false
			}
		}
	}

	if installType == ResourceInstallTypeNone {
		if getMeta != nil {
			return true
		}

		return false
	}

	return true
}

func mustDeleteOnFailedDeploy(res *resource.InstallableResource, getMeta *id.ResourceMeta, installType ResourceInstallType, releaseNamespace string) bool {
	if !res.DeleteOnFailed ||
		res.KeepOnDelete ||
		installType == ResourceInstallTypeNone {
		return false
	}

	if getMeta != nil {
		if err := resource.ValidateResourcePolicy(getMeta); err != nil {
			return false
		} else {
			if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
				return false
			}
		}
	}

	return true
}

func mustTrackReadiness(res *resource.InstallableResource, resInstallType ResourceInstallType, exists, prevRelFailed bool) bool {
	if resource.IsCRD(res.Unstruct.GroupVersionKind().GroupKind()) ||
		res.TrackTerminationMode == multitrack.NonBlocking {
		return false
	}

	if resInstallType == ResourceInstallTypeNone {
		if exists && prevRelFailed {
			return true
		}

		return false
	}

	return true
}
