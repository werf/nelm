package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/samber/lo"

	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/pkg/helm/pkg/storage/driver"
)

var _ Historier = (*History)(nil)

type Historier interface {
	Releases() []helmrel.Accessor
	FindAllDeployed() []helmrel.Accessor
	FindRevision(revision int) (helmrel.Accessor, bool)
	CreateRelease(ctx context.Context, rel helmrel.Accessor) error
	UpdateRelease(ctx context.Context, rel helmrel.Accessor) error
	DeleteRelease(ctx context.Context, name string, revision int) error
}

type History struct {
	releaseName string
	releases    []helmrel.Accessor
	storage     ReleaseStorager
	updateLock  sync.Mutex
}

func NewHistory(rels []helmrel.Accessor, releaseName string, historyStorage ReleaseStorager, opts HistoryOptions) *History {
	sort.SliceStable(rels, func(i, j int) bool {
		return rels[i].Version() < rels[j].Version()
	})

	return &History{
		releaseName: releaseName,
		releases:    rels,
		storage:     historyStorage,
	}
}

func (h *History) CreateRelease(ctx context.Context, rel helmrel.Accessor) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	if err := touchReleaseDeployTimes(rel.Releaser(), time.Now()); err != nil {
		return fmt.Errorf("touch release deploy times: %w", err)
	}

	if err := h.storage.Create(rel); err != nil {
		return fmt.Errorf("create release %q (namespace: %q, revision: %d): %w", rel.Name(), rel.Namespace(), rel.Version(), err)
	}

	h.releases = append(h.releases, rel)

	return nil
}

func (h *History) DeleteRelease(ctx context.Context, name string, revision int) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	rel, err := h.storage.Delete(name, revision)
	if err != nil {
		return fmt.Errorf("uninstall release %q (revision: %d): %w", name, revision, err)
	}

	if _, i, found := lo.FindIndexOf(h.releases, func(existing helmrel.Accessor) bool {
		return existing.Version() == rel.Version()
	}); !found {
		return nil
	} else {
		h.releases = slices.Delete(h.releases, i, i+1)
	}

	return nil
}

func (h *History) FindAllDeployed() []helmrel.Accessor {
	_, lastUninstalledRelIndex, lastUninstalledRelFound := lo.FindLastIndexOf(h.releases, func(r helmrel.Accessor) bool {
		return r.Status() == helmreleasecommon.StatusUninstalled.String() ||
			r.Status() == helmreleasecommon.StatusUninstalling.String()
	})

	var relsSinceUninstalled []helmrel.Accessor
	if lastUninstalledRelFound {
		if lastUninstalledRelIndex == len(h.releases)-1 {
			return nil
		}

		relsSinceUninstalled = h.releases[lastUninstalledRelIndex+1:]
	} else {
		relsSinceUninstalled = h.releases
	}

	return lo.Filter(relsSinceUninstalled, func(r helmrel.Accessor, _ int) bool {
		return r.Status() == helmreleasecommon.StatusDeployed.String() ||
			r.Status() == helmreleasecommon.StatusSuperseded.String()
	})
}

func (h *History) FindRevision(revision int) (helmrel.Accessor, bool) {
	return lo.Find(h.releases, func(r helmrel.Accessor) bool {
		return r.Version() == revision
	})
}

func (h *History) Releases() []helmrel.Accessor {
	return h.releases
}

func (h *History) UpdateRelease(ctx context.Context, rel helmrel.Accessor) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	if err := touchReleaseDeployTimes(rel.Releaser(), time.Now()); err != nil {
		return fmt.Errorf("touch release deploy times: %w", err)
	}

	if err := h.storage.Update(rel); err != nil {
		return fmt.Errorf("update release %q (namespace: %q, revision: %d): %w", rel.Name(), rel.Namespace(), rel.Version(), err)
	}

	if _, i, found := lo.FindIndexOf(h.releases, func(existing helmrel.Accessor) bool {
		return existing.Version() == rel.Version()
	}); !found {
		return fmt.Errorf("release %q (namespace: %q, revision: %d) not found in history", rel.Name(), rel.Namespace(), rel.Version())
	} else {
		h.releases[i] = rel
	}

	return nil
}

type HistoryOptions struct{}

func BuildHistories(historyStorage ReleaseStorager, opts HistoryOptions) ([]*History, error) {
	rels, err := historyStorage.Query(map[string]string{"owner": "helm"})
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, fmt.Errorf("query releases: %w", err)
	}

	releasesByNamespace := map[string]map[string][]helmrel.Accessor{}
	for _, rel := range rels {
		if releasesByNamespace[rel.Namespace()] == nil {
			releasesByNamespace[rel.Namespace()] = map[string][]helmrel.Accessor{}
		}

		if releasesByNamespace[rel.Namespace()][rel.Name()] == nil {
			releasesByNamespace[rel.Namespace()][rel.Name()] = []helmrel.Accessor{}
		}

		releasesByNamespace[rel.Namespace()][rel.Name()] = append(releasesByNamespace[rel.Namespace()][rel.Name()], rel)
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
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, fmt.Errorf("query releases for release %q: %w", releaseName, err)
	}

	return NewHistory(
		rels,
		releaseName,
		historyStorage,
		opts,
	), nil
}

func touchReleaseDeployTimes(releaser helmrel.Releaser, t time.Time) error {
	switch r := releaser.(type) {
	case *helmrelease.Release:
		r.Info.FirstDeployed = t
		r.Info.LastDeployed = t
	case *v2release.Release:
		r.Info.FirstDeployed = t
		r.Info.LastDeployed = t
	default:
		return fmt.Errorf("unexpected release type: %T", releaser)
	}

	return nil
}
