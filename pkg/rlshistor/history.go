package rlshistor

import (
	"context"
	"fmt"
	"sync"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"

	legacyRelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/3p-helm/pkg/storage/driver"

	"github.com/werf/nelm/pkg/rls"
)

var _ Historier = (*History)(nil)

func NewHistory(releaseName, releaseNamespace string, historyStorage LegacyStorage, opts HistoryOptions) (*History, error) {
	legacyRels, err := historyStorage.Query(map[string]string{"name": releaseName, "owner": "helm"})
	if err != nil && err != driver.ErrReleaseNotFound {
		return nil, fmt.Errorf("error querying releases for release %q (namespace: %q): %w", releaseName, releaseNamespace, err)
	}
	releaseutil.SortByRevision(legacyRels)

	return &History{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
		legacyReleases:   legacyRels,
		storage:          historyStorage,
		mapper:           opts.Mapper,
		discoveryClient:  opts.DiscoveryClient,
	}, nil
}

type HistoryOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type History struct {
	releaseName      string
	releaseNamespace string
	legacyReleases   []*legacyRelease.Release
	storage          LegacyStorage
	mapper           meta.ResettableRESTMapper
	discoveryClient  discovery.CachedDiscoveryInterface
	updateLock       sync.Mutex
}

func (h *History) Release(revision int) (rel *rls.Release, found bool, err error) {
	legacyRel, found := lo.Find(h.legacyReleases, func(r *legacyRelease.Release) bool {
		return r.Version == revision
	})
	if !found {
		return nil, false, nil
	}

	rel, err = rls.NewReleaseFromLegacyRelease(legacyRel, rls.ReleaseFromLegacyReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	})
	if err != nil {
		return nil, false, fmt.Errorf("error constructing release from legacy release: %w", err)
	}

	return rel, true, nil
}

func (h *History) LastRelease() (rel *rls.Release, found bool, err error) {
	if h.Empty() {
		return nil, false, nil
	}

	legacyRel := h.legacyReleases[len(h.legacyReleases)-1]

	rel, err = rls.NewReleaseFromLegacyRelease(legacyRel, rls.ReleaseFromLegacyReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	})
	if err != nil {
		return nil, false, fmt.Errorf("error constructing release from legacy release: %w", err)
	}

	return rel, true, nil
}

// Get last successfully deployed release since last attempt to uninstall release or from the beginning of history.
func (h *History) LastDeployedRelease() (rel *rls.Release, found bool, err error) {
	if h.Empty() {
		return nil, false, nil
	}

	var legacyRel *legacyRelease.Release
legacyRelLoop:
	for i := len(h.legacyReleases) - 1; i >= 0; i-- {
		switch h.legacyReleases[i].Info.Status {
		case legacyRelease.StatusDeployed,
			legacyRelease.StatusSuperseded:
			legacyRel = h.legacyReleases[i]
			break legacyRelLoop
		case legacyRelease.StatusUninstalled,
			legacyRelease.StatusUninstalling:
			break legacyRelLoop
		case legacyRelease.StatusUnknown,
			legacyRelease.StatusFailed,
			legacyRelease.StatusPendingInstall,
			legacyRelease.StatusPendingUpgrade,
			legacyRelease.StatusPendingRollback:
		}
	}

	if legacyRel == nil {
		return nil, false, nil
	}

	rel, err = rls.NewReleaseFromLegacyRelease(legacyRel, rls.ReleaseFromLegacyReleaseOptions{
		Mapper:          h.mapper,
		DiscoveryClient: h.discoveryClient,
	})
	if err != nil {
		return nil, false, fmt.Errorf("error constructing release from legacy release: %w", err)
	}

	return rel, true, nil
}

func (h *History) Empty() bool {
	return len(h.legacyReleases) == 0
}

func (h *History) CreateRelease(ctx context.Context, rel *rls.Release) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	legacyRel, err := rls.NewLegacyReleaseFromRelease(rel)
	if err != nil {
		return fmt.Errorf("error constructing legacy release from release: %w", err)
	}

	if err := h.storage.Create(legacyRel); err != nil {
		return fmt.Errorf("error creating release %q (namespace: %q, revision: %q): %w", legacyRel.Name, legacyRel.Namespace, legacyRel.Version, err)
	}

	h.legacyReleases = append(h.legacyReleases, legacyRel)

	return nil
}

func (h *History) UpdateRelease(ctx context.Context, rel *rls.Release) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	legacyRel, err := rls.NewLegacyReleaseFromRelease(rel)
	if err != nil {
		return fmt.Errorf("error constructing legacy release from release: %w", err)
	}

	if err := h.storage.Update(legacyRel); err != nil {
		return fmt.Errorf("error updating release %q (namespace: %q, revision: %q): %w", legacyRel.Name, legacyRel.Namespace, legacyRel.Version, err)
	}

	if _, i, found := lo.FindIndexOf(h.legacyReleases, func(r *legacyRelease.Release) bool {
		return r.Version == rel.Revision()
	}); !found {
		return fmt.Errorf("release %q (namespace: %q, revision: %q) not found in history", rel.Name(), rel.Namespace(), rel.Revision())
	} else {
		h.legacyReleases[i] = legacyRel
	}

	return nil
}

type LegacyStorage interface {
	Create(rls *legacyRelease.Release) error
	Update(rls *legacyRelease.Release) error
	Query(labels map[string]string) ([]*legacyRelease.Release, error)
}

type Historier interface {
	Release(revision int) (rel *rls.Release, found bool, err error)
	LastRelease() (rel *rls.Release, found bool, err error)
	LastDeployedRelease() (rel *rls.Release, found bool, err error)
	Empty() bool
	CreateRelease(ctx context.Context, rel *rls.Release) error
	UpdateRelease(ctx context.Context, rel *rls.Release) error
}
