package plan

import (
	"github.com/werf/nelm/internal/resource"
)

func InstallableResourceInfoSortByMustInstallHandler(r1, r2 *InstallableResourceInfo) bool {
	if r1.MustInstall != r2.MustInstall {
		return ResourceInstallTypeSortHandler(r1.MustInstall, r2.MustInstall)
	}

	return resource.InstallableResourceSortByStageAndWeightHandler(r1.LocalResource, r2.LocalResource)
}
