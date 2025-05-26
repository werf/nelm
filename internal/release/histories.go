package release

import (
	"fmt"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/3p-helm/pkg/storage/driver"
)

type BuildHistoriesOptions struct {
	DiscoveryClient discovery.CachedDiscoveryInterface
	Mapper          meta.ResettableRESTMapper
}

func BuildHistories(historyStorage LegacyStorage, opts BuildHistoriesOptions) ([]*History, error) {
	legacyRels, err := historyStorage.Query(map[string]string{"owner": "helm"})
	if err != nil && err != driver.ErrReleaseNotFound {
		return nil, fmt.Errorf("query releases: %w", err)
	}

	histories := make(map[string]*History)
	for _, legacyRelease := range legacyRels {
		id := legacyRelease.Namespace + "/" + legacyRelease.Name

		if _, ok := histories[id]; ok {
			histories[id].legacyReleases = append(histories[id].legacyReleases, legacyRelease)
		} else {
			histories[id] = &History{
				releaseName:      legacyRelease.Name,
				releaseNamespace: legacyRelease.Namespace,
				legacyReleases:   []*helmrelease.Release{legacyRelease},
				storage:          historyStorage,
				mapper:           opts.Mapper,
				discoveryClient:  opts.DiscoveryClient,
			}
		}
	}

	for _, history := range histories {
		releaseutil.SortByRevision(history.legacyReleases)
	}

	return lo.Values(histories), nil
}
