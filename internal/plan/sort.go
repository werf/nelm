package plan

import (
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/common"
)

func InstallableResourceInfoSortByMustInstallHandler(r1, r2 *InstallableResourceInfo) bool {
	if r1.MustInstall != r2.MustInstall {
		return ResourceInstallTypeSortHandler(r1.MustInstall, r2.MustInstall)
	}

	if r1.Stage != r2.Stage {
		return common.StagesSortHandler(r1.Stage, r2.Stage)
	}

	return resource.InstallableResourceSortByWeightHandler(r1.LocalResource, r2.LocalResource)
}

func InstallableResourceInfoSortByStageHandler(r1, r2 *InstallableResourceInfo) bool {
	if r1.Stage != r2.Stage {
		return common.StagesSortHandler(r1.Stage, r2.Stage)
	}

	return resource.InstallableResourceSortByWeightHandler(r1.LocalResource, r2.LocalResource)
}
