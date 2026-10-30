package action

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/release"
)

var _ release.ReleaseStorager = (*prunedRevisionStorager)(nil)

type prunedRevisionStorager struct {
	getErr         error
	prunedSet      map[int]bool
	revisions      []release.Revision
	undecodableSet map[int]bool
}

func (s *prunedRevisionStorager) Create(ctx context.Context, rls helmrel.Accessor) error {
	return nil
}

func (s *prunedRevisionStorager) Delete(ctx context.Context, name string, version int) error {
	return nil
}

func (s *prunedRevisionStorager) GetRelease(ctx context.Context, name string, version int) (helmrel.Accessor, error) {
	if s.prunedSet[version] {
		return nil, release.ErrReleaseNotFound
	}

	if s.undecodableSet[version] {
		return nil, fmt.Errorf("%w: rev %d", release.ErrReleaseUndecodable, version)
	}

	if s.getErr != nil {
		return nil, s.getErr
	}

	return newTestReleaseAccessorForAction(name, version, helmreleasestatus.StatusDeployed)
}

func (s *prunedRevisionStorager) LatestRevisions(ctx context.Context, opts release.LatestRevisionsOptions) ([]release.Revision, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) ListLatestSummaries(ctx context.Context, opts release.ListLatestSummariesOptions) ([]release.RevisionSummary, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) ListRevisionSummaries(ctx context.Context, name string, opts release.ListRevisionSummariesOptions) ([]release.RevisionSummary, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) LoadRevision(ctx context.Context, revision release.Revision) (helmrel.Accessor, error) {
	return s.GetRelease(ctx, revision.Name, revision.Version)
}

func (s *prunedRevisionStorager) LoadRevisionSummary(ctx context.Context, revision release.Revision) (*release.ReleaseSummary, error) {
	return nil, errors.ErrUnsupported
}

func (s *prunedRevisionStorager) Revisions(ctx context.Context, name string) ([]release.Revision, error) {
	return s.revisions, nil
}

func (s *prunedRevisionStorager) Update(ctx context.Context, rls helmrel.Accessor) error {
	return nil
}

func (s *prunedRevisionStorager) UpdateLabels(ctx context.Context, name string, version int, labels map[string]string) error {
	return nil
}

func TestLoadDeployedAndPendingReleases_KeepsOnlyDeployedAndPendingStatuses(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusSuperseded),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusPendingInstall),
			newTestDeployedRevision("myrelease", 3, helmreleasestatus.StatusFailed),
			newTestDeployedRevision("myrelease", 4, helmreleasestatus.StatusPendingUpgrade),
			newTestDeployedRevision("myrelease", 5, helmreleasestatus.StatusUninstalled),
			newTestDeployedRevision("myrelease", 6, helmreleasestatus.StatusPendingRollback),
			newTestDeployedRevision("myrelease", 7, helmreleasestatus.StatusDeployed),
		},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	rels, err := loadDeployedAndPendingReleases(ctx, history, nil)
	require.NoError(t, err)

	assert.Equal(t, []int{2, 4, 6, 7}, lo.Map(rels, func(r helmrel.Accessor, _ int) int { return r.Version() }))
}

func TestLoadDeployedAndPendingReleases_PropagatesOtherErrors(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		getErr: assert.AnError,
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
		},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	_, err = loadDeployedAndPendingReleases(ctx, history, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestLoadDeployedAndPendingReleases_SkipsPrunedRevision(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		prunedSet: map[int]bool{1: true},
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusDeployed),
		},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	rels, err := loadDeployedAndPendingReleasesSkippingPruned(ctx, history)
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Equal(t, 2, rels[0].Version())

	_, err = loadDeployedAndPendingReleases(ctx, history, nil)
	require.ErrorIs(t, err, release.ErrReleaseNotFound, "the strict loader does not tolerate a vanished revision")
}

func TestLoadDeployedAndPendingReleases_SkipsUnreadablePendingRevision(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		prunedSet: map[int]bool{2: true},
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusPendingUpgrade),
			newTestDeployedRevision("myrelease", 3, helmreleasestatus.StatusPendingUpgrade),
		},
		undecodableSet: map[int]bool{3: true},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	rels, err := loadDeployedAndPendingReleases(ctx, history, nil)
	require.NoError(t, err, "a stuck pending revision that cannot be read must not block the deploy")
	assert.Equal(t, []int{1}, lo.Map(rels, func(r helmrel.Accessor, _ int) int { return r.Version() }))

	rels, err = loadDeployedAndPendingReleasesSkippingPruned(ctx, history)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, lo.Map(rels, func(r helmrel.Accessor, _ int) int { return r.Version() }))
}

func TestLoadDeployedAndPendingReleases_UndecodableRevisionIsAnError(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusDeployed),
		},
		undecodableSet: map[int]bool{1: true},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	_, err = loadDeployedAndPendingReleases(ctx, history, nil)
	require.ErrorIs(t, err, release.ErrReleaseUndecodable, "a deployed revision that cannot be read must fail, not be silently left deployed")

	_, err = loadDeployedAndPendingReleasesSkippingPruned(ctx, history)
	require.ErrorIs(t, err, release.ErrReleaseUndecodable, "the pruned-tolerant loader skips only revisions that no longer exist")
}

func newTestDeployedRevision(name string, version int, status helmreleasestatus.Status) release.Revision {
	return release.Revision{
		Name:      name,
		Namespace: "mynamespace",
		Status:    status.String(),
		Version:   version,
	}
}

func newTestReleaseAccessorForAction(name string, version int, status helmreleasestatus.Status) (helmrel.Accessor, error) {
	acc, err := helmrel.NewAccessor(&helmrelease.Release{
		Name:      name,
		Namespace: "mynamespace",
		Version:   version,
		Info:      &helmrelease.Info{Status: status},
	})
	if err != nil {
		return nil, fmt.Errorf("new release accessor: %w", err)
	}

	return acc, nil
}
