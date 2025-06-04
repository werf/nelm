package resourceinfo

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
)

func BuildDeployableResourceInfos(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	standaloneCRDs []*resource.StandaloneCRD,
	hookResources []*resource.HookResource,
	generalResources []*resource.GeneralResource,
	prevReleaseHookResources []*resource.HookResource,
	prevReleaseGeneralResources []*resource.GeneralResource,
	kubeClient kube.KubeClienter,
	mapper meta.ResettableRESTMapper,
	parallelism int,
) (
	releaseNamespaceInfo *DeployableReleaseNamespaceInfo,
	standaloneCRDsInfos []*DeployableStandaloneCRDInfo,
	hookResourcesInfos []*DeployableHookResourceInfo,
	generalResourcesInfos []*DeployableGeneralResourceInfo,
	prevReleaseHookResourceInfos []*DeployablePrevReleaseHookResourceInfo,
	prevReleaseGeneralResourceInfos []*DeployablePrevReleaseGeneralResourceInfo,
	err error,
) {
	totalResourcesCount := len(standaloneCRDs) + len(hookResources) + len(generalResources) + +len(prevReleaseHookResources) + len(prevReleaseGeneralResources)

	routines := lo.Max([]int{len(standaloneCRDs) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	standaloneCRDsPool := pool.NewWithResults[*DeployableStandaloneCRDInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range standaloneCRDs {
		res := res
		standaloneCRDsPool.Go(func(ctx context.Context) (*DeployableStandaloneCRDInfo, error) {
			if info, err := NewDeployableStandaloneCRDInfo(ctx, res, releaseNamespace, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing standalone crd info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	routines = lo.Max([]int{len(hookResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	hookResourcesPool := pool.NewWithResults[*DeployableHookResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range hookResources {
		res := res
		hookResourcesPool.Go(func(ctx context.Context) (*DeployableHookResourceInfo, error) {
			if info, err := NewDeployableHookResourceInfo(ctx, res, releaseNamespace, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing hook resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	routines = lo.Max([]int{len(generalResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	generalResourcesPool := pool.NewWithResults[*DeployableGeneralResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range generalResources {
		res := res
		generalResourcesPool.Go(func(ctx context.Context) (*DeployableGeneralResourceInfo, error) {
			if info, err := NewDeployableGeneralResourceInfo(ctx, res, releaseNamespace, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing general resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	routines = lo.Max([]int{len(prevReleaseHookResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	prevReleaseHookResourcesPool := pool.NewWithResults[*DeployablePrevReleaseHookResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range prevReleaseHookResources {
		res := res
		prevReleaseHookResourcesPool.Go(func(ctx context.Context) (*DeployablePrevReleaseHookResourceInfo, error) {
			if info, err := NewDeployablePrevReleaseHookResourceInfo(ctx, res, releaseNamespace, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing hook resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	routines = lo.Max([]int{len(prevReleaseGeneralResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	prevReleaseGeneralResourcesPool := pool.NewWithResults[*DeployablePrevReleaseGeneralResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range prevReleaseGeneralResources {
		res := res
		prevReleaseGeneralResourcesPool.Go(func(ctx context.Context) (*DeployablePrevReleaseGeneralResourceInfo, error) {
			if info, err := NewDeployablePrevReleaseGeneralResourceInfo(ctx, res, releaseNamespace, kubeClient, mapper); err != nil {
				return nil, fmt.Errorf("error constructing general resource info: %w", err)
			} else {
				return info, nil
			}
		})
	}

	standaloneCRDsInfos, err = standaloneCRDsPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("error waiting for standalone crds pool: %w", err)
	}

	hookResourcesInfos, err = hookResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("error waiting for hook resources pool: %w", err)
	}

	generalResourcesInfos, err = generalResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("error waiting for general resources pool: %w", err)
	}

	prevReleaseHookResourceInfos, err = prevReleaseHookResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("error waiting for hook resources pool: %w", err)
	}

	prevReleaseGeneralResourceInfos, err = prevReleaseGeneralResourcesPool.Wait()
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("error waiting for general resources pool: %w", err)
	}

	sort.SliceStable(standaloneCRDsInfos, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(standaloneCRDsInfos[i].ResourceID, standaloneCRDsInfos[j].ResourceID)
	})

	sort.SliceStable(hookResourcesInfos, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(hookResourcesInfos[i].ResourceID, hookResourcesInfos[j].ResourceID)
	})

	sort.SliceStable(generalResourcesInfos, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(generalResourcesInfos[i].ResourceID, generalResourcesInfos[j].ResourceID)
	})

	sort.SliceStable(prevReleaseHookResourceInfos, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(prevReleaseHookResourceInfos[i].ResourceID, prevReleaseHookResourceInfos[j].ResourceID)
	})

	sort.SliceStable(prevReleaseGeneralResourceInfos, func(i, j int) bool {
		return resource.ResourceIDsSortHandler(prevReleaseGeneralResourceInfos[i].ResourceID, prevReleaseGeneralResourceInfos[j].ResourceID)
	})

	return releaseNamespaceInfo, standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos, prevReleaseHookResourceInfos, prevReleaseGeneralResourceInfos, nil
}
