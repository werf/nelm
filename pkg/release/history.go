package release

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/samber/lo"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
)

var _ Historier = (*History)(nil)

type Historier interface {
	CreateRelease(ctx context.Context, rel helmrel.Accessor) error
	UpdateRelease(ctx context.Context, rel helmrel.Accessor) error
	DeleteRelease(ctx context.Context, name string, revision int) error
}

type History struct {
	releaseName string
	revisions   []Revision
	storage     ReleaseStorager
	updateLock  sync.Mutex
}

func (h *History) CreateRelease(ctx context.Context, rel helmrel.Accessor) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	now := time.Now()
	rel.SetFirstDeployed(now)
	rel.SetLastDeployed(now)

	if err := h.storage.Create(rel); err != nil {
		return fmt.Errorf("create release %q (namespace: %q, revision: %d): %w", rel.Name(), rel.Namespace(), rel.Version(), err)
	}

	h.revisions = append(h.revisions, revisionFromAccessor(rel))

	return nil
}

func (h *History) DeleteRelease(ctx context.Context, name string, revision int) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	if _, err := h.storage.Delete(name, revision); err != nil {
		return fmt.Errorf("uninstall release %q (revision: %d): %w", name, revision, err)
	}

	_, i, found := lo.FindIndexOf(h.revisions, func(existing Revision) bool {
		return existing.Version == revision
	})
	if !found {
		return nil
	}

	h.revisions = slices.Delete(h.revisions, i, i+1)

	return nil
}

func (h *History) Release(ctx context.Context, version int) (helmrel.Accessor, error) {
	rel, err := h.storage.GetRelease(h.releaseName, version)
	if err != nil {
		return nil, fmt.Errorf("get release %q (revision: %d): %w", h.releaseName, version, err)
	}

	return rel, nil
}

func (h *History) Revisions() []Revision {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	return slices.Clone(h.revisions)
}

func (h *History) UpdateRelease(ctx context.Context, rel helmrel.Accessor) error {
	h.updateLock.Lock()
	defer h.updateLock.Unlock()

	now := time.Now()
	rel.SetFirstDeployed(now)
	rel.SetLastDeployed(now)

	if err := h.storage.Update(rel); err != nil {
		return fmt.Errorf("update release %q (namespace: %q, revision: %d): %w", rel.Name(), rel.Namespace(), rel.Version(), err)
	}

	_, i, found := lo.FindIndexOf(h.revisions, func(existing Revision) bool {
		return existing.Version == rel.Version()
	})
	if !found {
		return fmt.Errorf("release %q (namespace: %q, revision: %d) not found in history", rel.Name(), rel.Namespace(), rel.Version())
	}

	h.revisions[i] = revisionFromAccessor(rel)

	return nil
}

func BuildHistory(ctx context.Context, releaseName string, historyStorage ReleaseStorager) (*History, error) {
	revisions, err := historyStorage.Revisions(ctx, releaseName)
	if err != nil {
		return nil, fmt.Errorf("list revisions for release %q: %w", releaseName, err)
	}

	return &History{
		releaseName: releaseName,
		revisions:   revisions,
		storage:     historyStorage,
	}, nil
}

func revisionFromAccessor(rel helmrel.Accessor) Revision {
	return Revision{
		Name:      rel.Name(),
		Namespace: rel.Namespace(),
		Version:   rel.Version(),
		Status:    rel.Status(),
	}
}
