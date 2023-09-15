package plnbuilder

import (
	"helm.sh/helm/v3/pkg/werf/resrcinfo"
	"k8s.io/apimachinery/pkg/types"
)

func CurrentReleaseExistingResourcesUIDs(
	standaloneCRDsInfos []*resrcinfo.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*resrcinfo.DeployableHookResourceInfo,
	generalResourcesInfos []*resrcinfo.DeployableGeneralResourceInfo,
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
