package release

import (
	"fmt"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/storage/driver"
)

func BuildHistories(historyStorage ReleaseStorager, opts HistoryOptions) ([]*History, error) {
	rels, err := historyStorage.Query(map[string]string{"owner": "helm"})
	if err != nil && err != driver.ErrReleaseNotFound {
		return nil, fmt.Errorf("query releases: %w", err)
	}

	releasesByNamespace := map[string]map[string][]*helmrelease.Release{}
	for _, rel := range rels {
		if releasesByNamespace[rel.Namespace] == nil {
			releasesByNamespace[rel.Namespace] = map[string][]*helmrelease.Release{}
		}

		if releasesByNamespace[rel.Namespace][rel.Name] == nil {
			releasesByNamespace[rel.Namespace][rel.Name] = []*helmrelease.Release{}
		}

		releasesByNamespace[rel.Namespace][rel.Name] = append(releasesByNamespace[rel.Namespace][rel.Name], rel)
	}

	var histories []*History
	for _, releasesFromNamespace := range releasesByNamespace {
		for relName, revisions := range releasesFromNamespace {
			history := NewHistory(
				revisions,
				relName,
				historyStorage,
				opts,
			)

			histories = append(histories, history)
		}
	}

	return histories, nil
}
