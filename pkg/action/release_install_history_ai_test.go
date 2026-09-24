//go:build ai_tests

package action

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/helm/pkg/storage/driver"
	"github.com/werf/nelm/v2/pkg/release"
)

var _ release.ReleaseStorager = (*prunedRevisionStorager)(nil)

type prunedRevisionStorager struct {
	getErr    error
	prunedSet map[int]bool
	revisions []release.Revision
}

func (s *prunedRevisionStorager) Create(rls helmrel.Accessor) error {
	return nil
}

func (s *prunedRevisionStorager) Delete(name string, version int) (helmrel.Accessor, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) GetRelease(name string, version int) (helmrel.Accessor, error) {
	if s.prunedSet[version] {
		return nil, driver.ErrReleaseNotFound
	}

	if s.getErr != nil {
		return nil, s.getErr
	}

	return newTestReleaseAccessorForAction(name, version, helmreleasestatus.StatusDeployed)
}

func (s *prunedRevisionStorager) ListLatestReleases(ctx context.Context) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *prunedRevisionStorager) Revisions(ctx context.Context, name string) ([]release.Revision, error) {
	return s.revisions, nil
}

func (s *prunedRevisionStorager) Update(rls helmrel.Accessor) error {
	return nil
}

func (s *prunedRevisionStorager) UpdateLabels(name string, version int, labels map[string]string) error {
	return nil
}

func TestAI_LoadDeployedReleasesSkippingPruned_KeepsOnlyDeployedStatus(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusSuperseded),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusFailed),
			newTestDeployedRevision("myrelease", 3, helmreleasestatus.StatusDeployed),
		},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	rels, err := loadDeployedReleasesSkippingPruned(ctx, history)
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Equal(t, 3, rels[0].Version())
}

func TestAI_LoadDeployedReleasesSkippingPruned_PropagatesOtherErrors(t *testing.T) {
	ctx := context.Background()

	storage := &prunedRevisionStorager{
		getErr: assert.AnError,
		revisions: []release.Revision{
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
		},
	}

	history, err := release.BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	_, err = loadDeployedReleasesSkippingPruned(ctx, history)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestAI_LoadDeployedReleasesSkippingPruned_SkipsPrunedRevision(t *testing.T) {
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

	rels, err := loadDeployedReleasesSkippingPruned(ctx, history)
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Equal(t, 2, rels[0].Version())
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
	return helmrel.NewAccessor(&helmrelease.Release{
		Name:      name,
		Namespace: "mynamespace",
		Version:   version,
		Info:      &helmrelease.Info{Status: status},
	})
}
