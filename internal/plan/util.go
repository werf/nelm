package plan

import (
	"k8s.io/apimachinery/pkg/types"

	info "github.com/werf/nelm/internal/plan/resourceinfo"
)

func CurrentReleaseExistingResourcesUIDs(
	standaloneCRDsInfos []*info.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*info.DeployableHookResourceInfo,
	generalResourcesInfos []*info.DeployableGeneralResourceInfo,
) (existingUIDs []types.UID, present bool) {
	for _, info := range standaloneCRDsInfos {
		if uid, found := info.LiveUID(); found {
			existingUIDs = append(existingUIDs, uid)
		}
	}

	for _, info := range hookResourcesInfos {
		if uid, found := info.LiveUID(); found {
			existingUIDs = append(existingUIDs, uid)
		}
	}

	for _, info := range generalResourcesInfos {
		if uid, found := info.LiveUID(); found {
			existingUIDs = append(existingUIDs, uid)
		}
	}

	return existingUIDs, len(existingUIDs) > 0
}
