package resource

import (
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/id"
)

func InstallableResourceSortHandler(r1, r2 *InstallableResource) bool {
	if r1.Stage != r2.Stage {
		return common.StagesSortHandler(r1.Stage, r2.Stage)
	}

	if r1.Weight == nil {
		return true
	} else if r2.Weight == nil {
		return false
	} else if r1.Weight != r2.Weight {
		return *r1.Weight < *r2.Weight
	}

	return id.ResourceSpecSortHandler(r1.ResourceSpec, r2.ResourceSpec)
}
