package action

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/werf/logboek"

	"github.com/werf/nelm/v2/pkg/common"
	chartv2 "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/release"
)

var _ release.ReleaseStorager = (*listStorager)(nil)

type listStorager struct {
	loadErrs map[string]error
}

func (s *listStorager) Create(ctx context.Context, rls helmrel.Accessor) error {
	return nil
}

func (s *listStorager) Delete(ctx context.Context, name string, version int) error {
	return nil
}

func (s *listStorager) ForEachRelease(ctx context.Context, name string, fn func(revision release.Revision, rel helmrel.Accessor, err error) error) error {
	return nil
}

func (s *listStorager) GetRelease(ctx context.Context, name string, version int) (helmrel.Accessor, error) {
	return nil, errors.ErrUnsupported
}

func (s *listStorager) LatestRevisions(ctx context.Context) ([]release.Revision, error) {
	return nil, nil
}

func (s *listStorager) LoadRevision(ctx context.Context, revision release.Revision) (helmrel.Accessor, error) {
	if err := s.loadErrs[revision.Name]; err != nil {
		return nil, err
	}

	return helmrel.NewAccessor(&helmrelease.Release{
		Name:      revision.Name,
		Namespace: revision.Namespace,
		Version:   revision.Version,
		Info: &helmrelease.Info{
			Status:      helmreleasestatus.Status(revision.Status),
			Annotations: map[string]string{"managed-by": "test"},
		},
		Chart: &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}},
	})
}

func (s *listStorager) Revisions(ctx context.Context, name string) ([]release.Revision, error) {
	return nil, nil
}

func (s *listStorager) Update(ctx context.Context, rls helmrel.Accessor) error {
	return nil
}

func (s *listStorager) UpdateLabels(ctx context.Context, name string, version int, labels map[string]string) error {
	return nil
}

func TestReleaseListResultRelease_Load(t *testing.T) {
	rel := &ReleaseListResultRelease{Name: "a", Namespace: "ns", Revision: 3, Status: helmreleasestatus.StatusDeployed, storage: &listStorager{}}

	details, err := rel.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"managed-by": "test"}, details.Annotations)
	assert.Equal(t, &ReleaseListResultChart{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}, details.Chart)
	require.NotNil(t, details.DeployedAt)
}

func TestBuildReleaseListOutput_UndecodableReleaseIsShownWithoutDetails(t *testing.T) {
	storage := &listStorager{loadErrs: map[string]error{
		"broken": fmt.Errorf("%w: object %q", release.ErrReleaseUndecodable, "sh.helm.release.v1.broken.v2"),
	}}

	result := &ReleaseListResult{Releases: []*ReleaseListResultRelease{
		{Name: "a", Namespace: "ns", Revision: 1, Status: helmreleasestatus.StatusDeployed, storage: storage},
		{Name: "broken", Namespace: "ns", Revision: 2, Status: helmreleasestatus.StatusFailed, storage: storage},
	}}

	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))

	output, err := buildReleaseListOutput(ctx, result, 2)
	require.NoError(t, err)
	require.Len(t, output.Releases, 2)

	assert.Equal(t, "a", output.Releases[0].Name)
	assert.Equal(t, "mychart", output.Releases[0].Chart.Name)

	assert.Equal(t, &releaseListOutputRelease{Name: "broken", Namespace: "ns", Revision: 2, Status: helmreleasestatus.StatusFailed}, output.Releases[1])
}

func TestBuildReleaseListOutput_OtherLoadErrorsFail(t *testing.T) {
	storage := &listStorager{loadErrs: map[string]error{"a": errors.New("connection refused")}}

	result := &ReleaseListResult{Releases: []*ReleaseListResultRelease{
		{Name: "a", Namespace: "ns", Revision: 1, Status: helmreleasestatus.StatusDeployed, storage: storage},
	}}

	_, err := buildReleaseListOutput(context.Background(), result, 2)
	require.ErrorContains(t, err, "connection refused")
}

func TestBuildReleaseListOutput_NoReleases(t *testing.T) {
	output, err := buildReleaseListOutput(context.Background(), &ReleaseListResult{}, 2)
	require.NoError(t, err)
	assert.Nil(t, output.Releases, "an empty list is printed as null, as before")
}

func TestBuildReleaseHistoryOutputTable_RevisionWithoutDetails(t *testing.T) {
	result := &ReleaseHistoryResultV1{Releases: []*ReleaseHistoryResultRelease{
		{Name: "a", Namespace: "ns", Revision: 1, Status: helmreleasestatus.StatusFailed},
	}}

	require.NotPanics(t, func() {
		buildReleaseHistoryOutputTable(context.Background(), result).Render()
	})
}

func TestBuildReleaseListOutput_ReleaseRemovedDuringLoadIsSkipped(t *testing.T) {
	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))

	storage, err := release.NewReleaseStorage(ctx, "ns", common.ReleaseStorageDriverMemory, nil, release.ReleaseStorageOptions{})
	require.NoError(t, err)

	for _, name := range []string{"a", "b"} {
		acc, err := helmrel.NewAccessor(&helmrelease.Release{
			Name:      name,
			Namespace: "ns",
			Version:   1,
			Info:      &helmrelease.Info{Status: helmreleasestatus.StatusDeployed},
			Chart:     &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "mychart"}},
		})
		require.NoError(t, err)
		require.NoError(t, storage.Create(ctx, acc))
	}

	revisions, err := storage.LatestRevisions(ctx)
	require.NoError(t, err)

	result := &ReleaseListResult{}
	for _, revision := range revisions {
		result.Releases = append(result.Releases, &ReleaseListResultRelease{
			Name:      revision.Name,
			Namespace: revision.Namespace,
			Revision:  revision.Version,
			Status:    helmreleasestatus.Status(revision.Status),
			storage:   storage,
		})
	}

	require.NoError(t, storage.Delete(ctx, "b", 1))

	output, err := buildReleaseListOutput(ctx, result, 2)
	require.NoError(t, err)
	require.Len(t, output.Releases, 1)
	assert.Equal(t, "a", output.Releases[0].Name)

	require.NoError(t, storage.Delete(ctx, "a", 1))

	output, err = buildReleaseListOutput(ctx, result, 2)
	require.NoError(t, err)
	assert.Nil(t, output.Releases, "an empty list is printed as null")
}
