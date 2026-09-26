//go:build ai_tests

package release

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

var _ ReleaseStorager = (*stubStorager)(nil)

type stubStorager struct {
	deleteErr error
	revisions []Revision
}

func (s *stubStorager) Create(rls helmrel.Accessor) error {
	return nil
}

func (s *stubStorager) Delete(ctx context.Context, name string, version int) error {
	return s.deleteErr
}

func (s *stubStorager) GetRelease(name string, version int) (helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) ListLatestReleases(ctx context.Context) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) Query(labels map[string]string) ([]helmrel.Accessor, error) {
	return nil, nil
}

func (s *stubStorager) Revisions(ctx context.Context, name string) ([]Revision, error) {
	return s.revisions, nil
}

func (s *stubStorager) Update(rls helmrel.Accessor) error {
	return nil
}

func (s *stubStorager) UpdateLabels(name string, version int, labels map[string]string) error {
	return nil
}

func TestAI_DeleteRelease_ErrorIncludesNameAndRevision(t *testing.T) {
	ctx := context.Background()

	storage := &stubStorager{
		deleteErr: errors.New("kube delete failed"),
		revisions: []Revision{
			{Name: "myrelease", Namespace: testNamespace, Version: 3, Status: helmreleasecommon.StatusDeployed.String()},
		},
	}

	history, err := BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	var delErr error
	require.NotPanics(t, func() {
		delErr = history.DeleteRelease(ctx, "myrelease", 3)
	})

	require.Error(t, delErr)
	assert.ErrorIs(t, delErr, storage.deleteErr)
	assert.Contains(t, delErr.Error(), `"myrelease"`)
	assert.Contains(t, delErr.Error(), "revision: 3")
}

func TestAI_History_ReleaseLoadsBodyOnDemand(t *testing.T) {
	ctx := context.Background()

	storage := newMemoryReleaseStorage(t,
		newTestReleaseWithStatus("myrelease", 1, helmreleasecommon.StatusSuperseded),
		newTestReleaseWithStatus("myrelease", 2, helmreleasecommon.StatusDeployed),
	)

	history, err := BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	revisions := history.Revisions()
	require.Len(t, revisions, 2)
	assert.Equal(t, 1, revisions[0].Version)
	assert.Equal(t, 2, revisions[1].Version)

	rel, err := history.Release(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "myrelease", rel.Name())
	assert.Equal(t, 1, rel.Version())
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), rel.Status())
}

func TestAI_History_RevisionsSnapshotSurvivesDelete(t *testing.T) {
	ctx := context.Background()

	storage := newMemoryReleaseStorage(t,
		newTestReleaseWithStatus("myrelease", 1, helmreleasecommon.StatusSuperseded),
		newTestReleaseWithStatus("myrelease", 2, helmreleasecommon.StatusDeployed),
	)

	history, err := BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)

	revisions := history.Revisions()
	require.Len(t, revisions, 2)

	require.NoError(t, history.DeleteRelease(ctx, "myrelease", 1))

	assert.Equal(t, 1, revisions[0].Version, "captured revisions must not be mutated by DeleteRelease")
	assert.Equal(t, helmreleasecommon.StatusSuperseded.String(), revisions[0].Status)
	assert.Equal(t, 2, revisions[1].Version)
	assert.Equal(t, helmreleasecommon.StatusDeployed.String(), revisions[1].Status)
}

func TestAI_History_RevisionsStayConsistentAfterMutations(t *testing.T) {
	ctx := context.Background()

	storage := newMemoryReleaseStorage(t,
		newTestReleaseWithStatus("myrelease", 1, helmreleasecommon.StatusDeployed),
	)

	history, err := BuildHistory(ctx, "myrelease", storage)
	require.NoError(t, err)
	require.Len(t, history.Revisions(), 1)

	newRel := newTestReleaseAccessor(t, "myrelease", 2, helmreleasecommon.StatusPendingUpgrade)
	require.NoError(t, history.CreateRelease(ctx, newRel))

	revisions := history.Revisions()
	require.Len(t, revisions, 2)
	assert.Equal(t, 2, revisions[1].Version)
	assert.Equal(t, helmreleasecommon.StatusPendingUpgrade.String(), revisions[1].Status)

	newRel.SetStatus(helmreleasecommon.StatusDeployed)
	require.NoError(t, history.UpdateRelease(ctx, newRel))

	revisions = history.Revisions()
	require.Len(t, revisions, 2)
	assert.Equal(t, helmreleasecommon.StatusDeployed.String(), revisions[1].Status,
		"UpdateRelease must refresh the cached revision status")

	require.NoError(t, history.DeleteRelease(ctx, "myrelease", 1))

	revisions = history.Revisions()
	require.Len(t, revisions, 1)
	assert.Equal(t, 2, revisions[0].Version)
}
