package action

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	chartv2 "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/release"
)

func TestApplyReleaseListOptionsDefaultsRejectsInvalidSelector(t *testing.T) {
	_, err := applyReleaseListOptionsDefaults(ReleaseListOptions{TempDirPath: t.TempDir(), ReleaseLabelSelector: "team in ("}, t.TempDir())
	require.ErrorContains(t, err, "selector")
}

func TestBuildReleaseHistoryOutputTable_RevisionWithoutDetails(t *testing.T) {
	result := &ReleaseHistoryResultV1{Releases: []*ReleaseHistoryResultRelease{
		{Name: "a", Namespace: "ns", Revision: 1, Status: helmreleasestatus.StatusFailed},
	}}
	require.NotPanics(t, func() { buildReleaseHistoryOutputTable(context.Background(), result).Render() })
}

func TestBuildReleaseListResultBackendAndFinalErrors(t *testing.T) {
	backendErr := errors.New("connection refused")
	storage := &latestReleaseListStorager{err: backendErr}
	_, err := buildReleaseListResult(context.Background(), storage, "")
	require.ErrorIs(t, err, backendErr)

	storage = &latestReleaseListStorager{entries: []latestReleaseListEntry{{revision: release.Revision{Namespace: "ns", Name: "myrel", Version: 1}, err: backendErr}}}
	_, err = buildReleaseListResult(context.Background(), storage, "")
	require.ErrorIs(t, err, backendErr)
}

func TestBuildReleaseListResultEmpty(t *testing.T) {
	result, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{}, "")
	require.NoError(t, err)
	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiVersion":"v1","releases":null}`, string(data))
}

func TestBuildReleaseListResultMalformedFinalChart(t *testing.T) {
	for _, chart := range []*chartv2.Chart{nil, {}} {
		entry := newLatestReleaseListEntry(t, "ns", "foo", 1)
		entry.rel.Releaser().(*helmrelease.Release).Chart = chart
		_, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{entries: []latestReleaseListEntry{entry}}, "")
		require.ErrorContains(t, err, "chart")
	}
}

func TestBuildReleaseListResultMalformedProvisionalChart(t *testing.T) {
	for _, chart := range []*chartv2.Chart{nil, {}} {
		old := newLatestReleaseListEntry(t, "ns", "foo", 1)
		old.rel.Releaser().(*helmrelease.Release).Chart = chart
		storage := &latestReleaseListStorager{entries: []latestReleaseListEntry{
			old, newLatestReleaseListEntry(t, "ns", "foo.v1a", 1), newLatestReleaseListEntry(t, "ns", "foo", 2),
		}}
		result, err := buildReleaseListResult(context.Background(), storage, "")
		require.NoError(t, err)
		require.Len(t, result.Releases, 2)
		assert.Equal(t, 2, result.Releases[0].Revision)
		require.NotNil(t, result.Releases[0].Chart)
	}
}

func TestBuildReleaseListResultProjectsAndSorts(t *testing.T) {
	storage := &latestReleaseListStorager{entries: []latestReleaseListEntry{
		newLatestReleaseListEntry(t, "z", "same", 3),
		newLatestReleaseListEntry(t, "a", "same", 1),
		newLatestReleaseListEntry(t, "a", "other", 1),
		newLatestReleaseListEntry(t, "a", "same", 10),
	}}
	result, err := buildReleaseListResult(context.Background(), storage, "packageChecksum")
	require.NoError(t, err)
	require.Len(t, result.Releases, 3)
	assert.Equal(t, "packageChecksum", storage.selector)
	assert.Equal(t, "other", result.Releases[0].Name)
	assert.Equal(t, "a", result.Releases[1].Namespace)
	assert.Equal(t, 10, result.Releases[1].Revision)
	assert.Equal(t, "z", result.Releases[2].Namespace)

	for _, rel := range result.Releases {
		assert.Equal(t, map[string]string{"managed-by": "deckhouse"}, rel.Annotations)
		assert.Equal(t, &ReleaseListResultChart{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}, rel.Chart)
		require.NotNil(t, rel.DeployedAt)
	}
}

func TestBuildReleaseListResultSupersedesProvisionalError(t *testing.T) {
	storage := &latestReleaseListStorager{entries: []latestReleaseListEntry{
		{revision: release.Revision{Namespace: "ns", Name: "myrel", Version: 1}, err: errors.New("provisional error")},
		newLatestReleaseListEntry(t, "ns", "myrel", 2),
	}}
	result, err := buildReleaseListResult(context.Background(), storage, "")
	require.NoError(t, err)
	require.Len(t, result.Releases, 1)
	assert.Equal(t, 2, result.Releases[0].Revision)
	require.NotNil(t, result.Releases[0].Chart)
}

func TestBuildReleaseListResultUndecodableLatest(t *testing.T) {
	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	storage := &latestReleaseListStorager{entries: []latestReleaseListEntry{
		newLatestReleaseListEntry(t, "ns", "myrel", 1),
		{revision: release.Revision{Namespace: "ns", Name: "myrel", Version: 2, Status: "failed"}, err: release.ErrReleaseUndecodable},
	}}
	result, err := buildReleaseListResult(ctx, storage, "")
	require.NoError(t, err)
	require.Len(t, result.Releases, 1)
	assert.Equal(t, &ReleaseListResultRelease{Name: "myrel", Namespace: "ns", Revision: 2, Status: helmreleasestatus.StatusFailed}, result.Releases[0])
}
