package resourceinfo

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

func BuildResourceInfos(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	instResources []*resource.InstallableResource,
	delResources []*resource.DeletableResource,
	prevReleaseFailed bool,
	kubeClient kube.KubeClienter,
	mapper meta.ResettableRESTMapper,
	parallelism int,
) (
	instResourceInfos []*InstallableResourceInfo,
	delResourceInfos []*DeletableResourceInfo,
	err error,
) {
	totalResourcesCount := len(instResources) + len(delResources)

	routines := lo.Max([]int{len(instResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	instResourcesPool := pool.NewWithResults[*InstallableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range instResources {
		res := res
		instResourcesPool.Go(func(ctx context.Context) (*InstallableResourceInfo, error) {
			info, err := BuildInstallableResourceInfo(ctx, res, releaseNamespace, prevReleaseFailed, kubeClient, mapper)
			if err != nil {
				return nil, fmt.Errorf("build installable resource info: %w", err)
			}

			return info, nil
		})
	}

	routines = lo.Max([]int{len(delResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	delResourcesPool := pool.NewWithResults[*DeletableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range delResources {
		res := res
		delResourcesPool.Go(func(ctx context.Context) (*DeletableResourceInfo, error) {
			info, err := BuildDeletableResourceInfo(ctx, res, releaseName, releaseNamespace, kubeClient, mapper)
			if err != nil {
				return nil, fmt.Errorf("build deletable resource info: %w", err)
			}

			return info, nil
		})
	}

	instResourceInfos, err = instResourcesPool.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("wait for resource pool: %w", err)
	}

	delResourceInfos, err = delResourcesPool.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("wait for prev release resource pool: %w", err)
	}

	sort.SliceStable(instResourceInfos, func(i, j int) bool {
		return resource.InstallableResourceSortHandler(instResourceInfos[i].LocalResource, instResourceInfos[j].LocalResource)
	})

	sort.SliceStable(delResourceInfos, func(i, j int) bool {
		return id.ResourceMetaSortHandler(delResourceInfos[i].LocalResource.ResourceMeta, delResourceInfos[j].LocalResource.ResourceMeta)
	})

	iterateInstallableResourceInfos(instResourceInfos)

	delResourceInfos = filterDelResourcesPresentInInstResources(instResourceInfos, delResourceInfos)
	delResourceInfos = deduplicateDeletableResourceInfos(delResourceInfos)

	return instResourceInfos, delResourceInfos, nil
}

func filterDelResourcesPresentInInstResources(instResourceInfos []*InstallableResourceInfo, delResourceInfos []*DeletableResourceInfo) []*DeletableResourceInfo {
	var instResourcesUIDs []types.UID
	for _, instInfo := range instResourceInfos {
		if instInfo.GetResult == nil {
			continue
		}

		instResourcesUIDs = append(instResourcesUIDs, instInfo.GetResult.GetUID())
	}

	var filteredDelResourceInfos []*DeletableResourceInfo
	for _, delInfo := range delResourceInfos {
		if delInfo.GetResult != nil &&
			lo.Contains(instResourcesUIDs, delInfo.GetResult.GetUID()) {
			continue
		}

		filteredDelResourceInfos = append(filteredDelResourceInfos, delInfo)
	}

	return filteredDelResourceInfos
}

func iterateInstallableResourceInfos(infos []*InstallableResourceInfo) {
	var seenInfos []*InstallableResourceInfo

	for _, info := range infos {
		seenInfo, seen := lo.Find(seenInfos, func(inf *InstallableResourceInfo) bool {
			return info.ID() == inf.ID()
		})
		if seen {
			info.Iteration = seenInfo.Iteration + 1
		}

		seenInfos = append(seenInfos, info)
	}
}

func deduplicateDeletableResourceInfos(infos []*DeletableResourceInfo) []*DeletableResourceInfo {
	return lo.UniqBy(infos, func(info *DeletableResourceInfo) string {
		return info.ID()
	})
}
