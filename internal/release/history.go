package release

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/3p-helm/pkg/storage/driver"
	helmtime "github.com/werf/3p-helm/pkg/time"
)

var _ Historier = (*History)(nil)

type Historier interface {
	Releases() []*helmrelease.Release
	FindAllDeployed() []*helmrelease.Release
	FindRevision(revision int) (rel *helmrelease.Release, found bool)
	CreateRelease(ctx context.Context, rel *helmrelease.Release) error
	UpdateRelease(ctx context.Context, rel *helmrelease.Release) error
	DeleteRelease(ctx context.Context, name string, revision int) error
}

type History struct {
	releaseName string
	releases    []*helmrelease.Release
	storage     ReleaseStorager
	updateLock  sync.Mutex
}

type HistoryOptions struct{}

func NewHistory(rels []*helmrelease.Release, releaseName string, historyStorage ReleaseStorager, opts HistoryOptions) *History {
	releaseutil.SortByRevision(rels)

	return &History{
		releaseName: releaseName,
		releases:    rels,
		storage:     historyStorage,
	}
}

func (h *History) Releases() []*helmrelease.Release {
	return h.releases
}

func (h *History) FindAllDeployed() []*helmrelease.Release {
	_, lastUninstalledRelIndex, lastUninstalledRelFound := lo.FindLastIndexOf(h.releases, func(r *helmrelease.Release) bool {
		return r.Info.Status == helmrelease.StatusUninstalled ||
			r.Info.Status == helmrelease.StatusUninstalling
	})

	var relsSinceUninstalled []*helmrelease.Release
	if lastUninstalledRelFound {
		if lastUninstalledRelIndex == len(h.releases)-1 {
			return nil
		}

		relsSinceUninstalled = h.releases[lastUninstalledRelIndex+1:]
	} else {
		relsSinceUninstalled = h.releases
	}

	return lo.Filter(relsSinceUninstalled, func(r *helmrelease.Release, _ int) bool {
		return r.Info.Status == helmrelease.StatusDeployed ||
			r.Info.Status == helmrelease.StatusSuperseded
	})
}

func (h *History) FindRevision(revision int) (rel *helmrelease.Release, found bool) {
	return lo.Find(h.releases, func(r *helmrelease.Release) bool {
		return r.Version == revision
	})
}

func (h *History) CreateRelease(ctx context.Context, rel *helmrelease.Release) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	rel.Info.FirstDeployed = helmtime.Now()
	rel.Info.LastDeployed = rel.Info.FirstDeployed

	if err := h.storage.Create(rel); err != nil {
		return fmt.Errorf("create release %q (namespace: %q, revision: %q): %w", rel.Name, rel.Namespace, rel.Version, err)
	}

	h.releases = append(h.releases, rel)

	return nil
}

func (h *History) UpdateRelease(ctx context.Context, rel *helmrelease.Release) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	rel.Info.FirstDeployed = helmtime.Now()
	rel.Info.LastDeployed = rel.Info.FirstDeployed

	if err := h.storage.Update(rel); err != nil {
		return fmt.Errorf("update release %q (namespace: %q, revision: %q): %w", rel.Name, rel.Namespace, rel.Version, err)
	}

	if _, i, found := lo.FindIndexOf(h.releases, func(r *helmrelease.Release) bool {
		return r.Version == rel.Version
	}); !found {
		return fmt.Errorf("release %q (namespace: %q, revision: %q) not found in history", rel.Name, rel.Namespace, rel.Version)
	} else {
		h.releases[i] = rel
	}

	return nil
}

func (h *History) DeleteRelease(ctx context.Context, name string, revision int) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	rel, err := h.storage.Delete(name, revision)
	if err != nil {
		return fmt.Errorf("uninstall release %q (namespace: %q, revision: %q): %w", rel.Name, rel.Namespace, rel.Version, err)
	}

	if _, i, found := lo.FindIndexOf(h.releases, func(r *helmrelease.Release) bool {
		return r.Version == rel.Version
	}); !found {
		return nil
	} else {
		h.releases = slices.Delete(h.releases, i, i+1)
	}

	return nil
}

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

func BuildHistory(releaseName string, historyStorage ReleaseStorager, opts HistoryOptions) (*History, error) {
	rels, err := historyStorage.Query(map[string]string{"name": releaseName, "owner": "helm"})
	if err != nil && err != driver.ErrReleaseNotFound {
		return nil, fmt.Errorf("query releases for release %q: %w", releaseName, err)
	}

	return NewHistory(
		rels,
		releaseName,
		historyStorage,
		opts,
	), nil
}
