package resrcinfo

import (
	"context"
	"fmt"

	"github.com/sourcegraph/conc/pool"
	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"k8s.io/apimachinery/pkg/api/meta"
)

func BuildDeployableResourceInfos(
	ctx context.Context,
	releaseName string,
	releaseNamespace *resrc.ReleaseNamespace,
	standaloneCRDs []*resrc.StandaloneCRD,
	hookResources []*resrc.HookResource,
	generalResources []*resrc.GeneralResource,
	prevReleaseGeneralResources []*resrc.GeneralResource,
	kubeClient kubeclnt.KubeClienter,
	mapper meta.ResettableRESTMapper,
	parallelism int,
) (
	releaseNamespaceInfo *DeployableReleaseNamespaceInfo,
	standaloneCRDsInfos []*DeployableStandaloneCRDInfo,
	hookResourcesInfos []*DeployableHookResourceInfo,
	generalResourcesInfos []*DeployableGeneralResourceInfo,
	prevReleaseGeneralResourceInfos []*DeployablePrevReleaseGeneralResourceInfo,
	err error,
) {
	releaseNamespaceInfo, err = NewDeployableReleaseNamespaceInfo(ctx, releaseNamespace, kubeClient, mapper)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error creating new release namespace info: %w", err)
	}

	standaloneCRDsPool := pool.NewWithResults[*DeployableStandaloneCRDInfo]().WithContext(ctx).WithMaxGoroutines(parallelism).WithCancelOnError().WithFirstError()
	for _, res := range standaloneCRDs {
		standaloneCRDsPool.Go(func(ctx context.Context) (*DeployableStandaloneCRDInfo, error) {
			if info, err := NewDeployableStandaloneCRDInfo(ctx, res, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing standalone crd info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	standaloneCRDsInfos, err = standaloneCRDsPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error waiting for standalone crds pool: %w", err)
	}

	hookResourcesPool := pool.NewWithResults[*DeployableHookResourceInfo]().WithContext(ctx).WithMaxGoroutines(parallelism).WithCancelOnError().WithFirstError()
	for _, res := range hookResources {
		hookResourcesPool.Go(func(ctx context.Context) (*DeployableHookResourceInfo, error) {
			if info, err := NewDeployableHookResourceInfo(ctx, res, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing hook resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	hookResourcesInfos, err = hookResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error waiting for hook resources pool: %w", err)
	}

	generalResourcesPool := pool.NewWithResults[*DeployableGeneralResourceInfo]().WithContext(ctx).WithMaxGoroutines(parallelism).WithCancelOnError().WithFirstError()
	for _, res := range generalResources {
		generalResourcesPool.Go(func(ctx context.Context) (*DeployableGeneralResourceInfo, error) {
			if info, err := NewDeployableGeneralResourceInfo(ctx, res, releaseName, releaseNamespace.Name(), kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing general resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	generalResourcesInfos, err = generalResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("error waiting for general resources pool: %w", err)
	}

	if len(prevReleaseGeneralResources) > 0 {
		prevReleaseGeneralResourcesPool := pool.NewWithResults[*DeployablePrevReleaseGeneralResourceInfo]().WithContext(ctx).WithMaxGoroutines(parallelism).WithCancelOnError().WithFirstError()
		for _, res := range prevReleaseGeneralResources {
			prevReleaseGeneralResourcesPool.Go(func(ctx context.Context) (*DeployablePrevReleaseGeneralResourceInfo, error) {
				if info, err := NewDeployablePrevReleaseGeneralResourceInfo(ctx, res, kubeClient, mapper); err != nil {
					return nil, fmt.Errorf("error constructing general resource info: %w", err)
				} else {
					return info, nil
				}
			})
		}

		prevReleaseGeneralResourceInfos, err = prevReleaseGeneralResourcesPool.Wait()
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("error waiting for general resources pool: %w", err)
		}
	}

	return releaseNamespaceInfo, standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos, prevReleaseGeneralResourceInfos, nil
}
