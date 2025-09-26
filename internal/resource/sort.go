package resource

import (
	"github.com/werf/nelm/internal/resource/spec"
)

func InstallableResourceSortByWeightHandler(r1, r2 *InstallableResource) bool {
	if r1.Weight == nil {
		return true
	} else if r2.Weight == nil {
		return false
	} else if r1.Weight != r2.Weight {
		return *r1.Weight < *r2.Weight
	}

	return spec.ResourceSpecSortHandler(r1.ResourceSpec, r2.ResourceSpec)
}
