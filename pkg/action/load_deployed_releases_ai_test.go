//go:build ai_tests

package action

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/release"
)

var _ release.ReleaseStorager = (*countingStorager)(nil)

type countingStorager struct {
	gets      []int
	revisions []release.Revision
}

func (s *countingStorager) Create(rls helmrel.Accessor) error {
	return nil
}

func (s *countingStorager) Delete(name string, version int) error {
	return nil
}

func (s *countingStorager) GetRelease(name string, version int) (helmrel.Accessor, error) {
	s.gets = append(s.gets, version)

	return newTestReleaseAccessorForAction(name, version, helmreleasestatus.StatusDeployed)
}

func (s *countingStorager) ListLatestReleases(ctx context.Context) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *countingStorager) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *countingStorager) Revisions(ctx context.Context, name string) ([]release.Revision, error) {
	return s.revisions, nil
}

func (s *countingStorager) Update(rls helmrel.Accessor) error {
	return nil
}

func (s *countingStorager) UpdateLabels(name string, version int, labels map[string]string) error {
	return nil
}

func TestAI_LoadDeployedReleases_FetchesWhatIsNotPreloaded(t *testing.T) {
	revisions := []release.Revision{
		newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
		newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusDeployed),
	}

	prevRelease, err := newTestReleaseAccessorForAction("myrelease", 2, helmreleasestatus.StatusDeployed)
	require.NoError(t, err)

	storage := &countingStorager{}
	rels, err := loadDeployedReleases(context.Background(), newTestHistory(t, storage, revisions), []helmrel.Accessor{prevRelease})
	require.NoError(t, err)

	require.Len(t, rels, 2)
	assert.Equal(t, []int{1}, storage.gets, "only the revision that was not preloaded is fetched")
	assert.Same(t, prevRelease, rels[1])
}

func TestAI_LoadDeployedReleases_IgnoresNilAndIrrelevantPreloaded(t *testing.T) {
	revisions := []release.Revision{
		newTestDeployedRevision("myrelease", 5, helmreleasestatus.StatusDeployed),
	}

	unrelated, err := newTestReleaseAccessorForAction("myrelease", 9, helmreleasestatus.StatusDeployed)
	require.NoError(t, err)

	storage := &countingStorager{}
	rels, err := loadDeployedReleases(context.Background(), newTestHistory(t, storage, revisions), []helmrel.Accessor{nil, unrelated})
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Equal(t, 5, rels[0].Version())
	assert.Equal(t, []int{5}, storage.gets, "a nil or non-matching preloaded body must not satisfy a revision")
}

// Preloading is purely a fetch optimization: the set of revisions handed to BuildReleaseInfos must
// be byte-for-byte the same as when every body is fetched from storage.
func TestAI_LoadDeployedReleases_PreloadingDoesNotChangeTheSet(t *testing.T) {
	for name, revisions := range loadDeployedReleasesTestCases() {
		t.Run(name, func(t *testing.T) {
			coldStorage := &countingStorager{}
			cold, err := loadDeployedReleases(context.Background(), newTestHistory(t, coldStorage, revisions), nil)
			require.NoError(t, err)

			lastRevision, _ := lo.Last(revisions)
			prevRelease, err := newTestReleaseAccessorForAction("myrelease", lastRevision.Version, helmreleasestatus.Status(lastRevision.Status))
			require.NoError(t, err)

			warmStorage := &countingStorager{}
			warm, err := loadDeployedReleases(context.Background(), newTestHistory(t, warmStorage, revisions), []helmrel.Accessor{prevRelease})
			require.NoError(t, err)

			assert.Equal(t,
				lo.Map(cold, func(r helmrel.Accessor, _ int) int { return r.Version() }),
				lo.Map(warm, func(r helmrel.Accessor, _ int) int { return r.Version() }),
				"preloading must not change which revisions are returned")
		})
	}
}

func TestAI_LoadDeployedReleases_ReusesPreloadedBodyInsteadOfFetching(t *testing.T) {
	revisions := []release.Revision{
		newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusSuperseded),
		newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusDeployed),
	}

	prevRelease, err := newTestReleaseAccessorForAction("myrelease", 2, helmreleasestatus.StatusDeployed)
	require.NoError(t, err)

	storage := &countingStorager{}
	rels, err := loadDeployedReleases(context.Background(), newTestHistory(t, storage, revisions), []helmrel.Accessor{prevRelease})
	require.NoError(t, err)

	require.Len(t, rels, 1)
	assert.Same(t, prevRelease, rels[0], "the already-loaded accessor must be reused")
	assert.Empty(t, storage.gets, "no revision should be fetched when its body is preloaded")
}

func loadDeployedReleasesTestCases() map[string][]release.Revision {
	return map[string][]release.Revision{
		"steady state upgrade": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusSuperseded),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusSuperseded),
			newTestDeployedRevision("myrelease", 3, helmreleasestatus.StatusDeployed),
		},
		"last revision superseded": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusSuperseded),
		},
		"last revision uninstalled": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusUninstalled),
		},
		"last revision failed": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusFailed),
		},
		"multiple deployed": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusDeployed),
			newTestDeployedRevision("myrelease", 2, helmreleasestatus.StatusDeployed),
		},
		"no deployed at all": {
			newTestDeployedRevision("myrelease", 1, helmreleasestatus.StatusFailed),
		},
		"empty history": {},
	}
}

func newTestHistory(t *testing.T, storage *countingStorager, revisions []release.Revision) *release.History {
	t.Helper()

	storage.revisions = revisions

	history, err := release.BuildHistory(context.Background(), "myrelease", storage)
	require.NoError(t, err)

	return history
}
