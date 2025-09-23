package resource

import (
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/meta"
)

func InstallableResourceSortByWeightHandler(r1, r2 *InstallableResource) bool {
	if r1.Weight == nil {
		return true
	} else if r2.Weight == nil {
		return false
	} else if r1.Weight != r2.Weight {
		return *r1.Weight < *r2.Weight
	}

	return ResourceSpecSortHandler(r1.ResourceSpec, r2.ResourceSpec)
}

func ResourceSpecSortHandler(r1, r2 *ResourceSpec) bool {
	sortAs1 := r1.StoreAs
	sortAs2 := r2.StoreAs
	// TODO(v2): sorted based on sortAs for compatibility. In future should just probably sort
	// like this: first CRDs (any type), then helm.sh/hook hooks, then the rest
	if sortAs1 != sortAs2 {
		if sortAs1 == common.StoreAsNone {
			return true
		} else if sortAs1 == common.StoreAsHook && sortAs2 != common.StoreAsNone {
			return true
		} else {
			return false
		}
	}

	return meta.ResourceMetaSortHandler(r1.ResourceMeta, r2.ResourceMeta)
}
