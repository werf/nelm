//go:build ai_tests

package action //nolint:testpackage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/helm/pkg/chart"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
	helmstorage "github.com/werf/nelm/pkg/helm/pkg/storage"
	"github.com/werf/nelm/pkg/helm/pkg/storage/driver"
	helmtime "github.com/werf/nelm/pkg/helm/pkg/time"
)

var _ driver.Driver = (*stubQueryDriver)(nil)

// stubQueryDriver answers Query from a fixed "<name>/<version>" keyed set of
// releases. Unlike the memory driver it can hold the same release name in
// several namespaces, which is what the all-namespaces listing has to handle.
type stubQueryDriver struct {
	byQuery map[string][]*helmrelease.Release
	err     error
}

func (d *stubQueryDriver) Create(_ string, _ *helmrelease.Release) error {
	panic("not used by the release listing")
}

func (d *stubQueryDriver) Delete(_ string) (*helmrelease.Release, error) {
	panic("not used by the release listing")
}

func (d *stubQueryDriver) Get(_ string) (*helmrelease.Release, error) {
	panic("not used by the release listing")
}

func (d *stubQueryDriver) List(_ func(*helmrelease.Release) bool) ([]*helmrelease.Release, error) {
	panic("not used by the release listing")
}

func (d *stubQueryDriver) Name() string {
	return "StubQuery"
}

func (d *stubQueryDriver) Query(labels map[string]string) ([]*helmrelease.Release, error) {
	if d.err != nil {
		return nil, d.err
	}

	rels, found := d.byQuery[labels["name"]+"/"+labels["version"]]
	if !found {
		return nil, driver.ErrReleaseNotFound
	}

	return rels, nil
}

func (d *stubQueryDriver) Update(_ string, _ *helmrelease.Release) error {
	panic("not used by the release listing")
}

func TestAI_BuildReleaseListResultReleases(t *testing.T) {
	storage := helmstorage.Init(&stubQueryDriver{byQuery: map[string][]*helmrelease.Release{
		"one/3": {{Name: "one", Namespace: "ns-a", Version: 3}},
		"two/1": {{Name: "two", Namespace: "ns-b", Version: 1}},
	}})

	releases, err := buildReleaseListResultReleases(context.Background(), storage, []driver.ReleaseMeta{
		{Name: "one", Namespace: "ns-a", Version: 3},
		{Name: "two", Namespace: "ns-b", Version: 1},
		{Name: "trimmed", Namespace: "ns-a", Version: 5},
	}, 4)
	require.NoError(t, err)

	require.Len(t, releases, 2, "a revision trimmed between listing and fetching must be skipped, not fail the listing")
	assert.ElementsMatch(t, []string{"one", "two"}, []string{releases[0].Name, releases[1].Name})
}

func TestAI_BuildReleaseListResultReleasesPropagatesError(t *testing.T) {
	queryErr := errors.New("query failed")
	storage := helmstorage.Init(&stubQueryDriver{err: queryErr})

	_, err := buildReleaseListResultReleases(context.Background(), storage, []driver.ReleaseMeta{
		{Name: "one", Namespace: "ns-a", Version: 3},
	}, 4)
	require.ErrorIs(t, err, queryErr)
}

func TestAI_GetReleaseRevisionAcceptsSingleReleaseWithoutNamespace(t *testing.T) {
	storage := helmstorage.Init(&stubQueryDriver{byQuery: map[string][]*helmrelease.Release{
		"one/3": {{Name: "one", Version: 3}},
	}})

	rel, err := getReleaseRevision(storage, driver.ReleaseMeta{Name: "one", Namespace: "ns-a", Version: 3})
	require.NoError(t, err)
	assert.Equal(t, 3, rel.Version)
}

func TestAI_GetReleaseRevisionNotFound(t *testing.T) {
	storage := helmstorage.Init(&stubQueryDriver{byQuery: map[string][]*helmrelease.Release{
		"one/3": {
			{Name: "one", Namespace: "ns-b", Version: 3},
			{Name: "one", Namespace: "ns-c", Version: 3},
		},
	}})

	_, err := getReleaseRevision(storage, driver.ReleaseMeta{Name: "one", Namespace: "ns-a", Version: 3})
	require.ErrorIs(t, err, driver.ErrReleaseNotFound)

	_, err = getReleaseRevision(storage, driver.ReleaseMeta{Name: "absent", Namespace: "ns-a", Version: 1})
	require.ErrorIs(t, err, driver.ErrReleaseNotFound)
}

func TestAI_GetReleaseRevisionPicksMatchingNamespace(t *testing.T) {
	storage := helmstorage.Init(&stubQueryDriver{byQuery: map[string][]*helmrelease.Release{
		"one/3": {
			{Name: "one", Namespace: "ns-b", Version: 3},
			{Name: "one", Namespace: "ns-a", Version: 3},
		},
	}})

	rel, err := getReleaseRevision(storage, driver.ReleaseMeta{Name: "one", Namespace: "ns-a", Version: 3})
	require.NoError(t, err)
	assert.Equal(t, "ns-a", rel.Namespace)
}

func TestAI_LastReleaseRevisions(t *testing.T) {
	metas := lastReleaseRevisions([]driver.ReleaseMeta{
		{Name: "one", Namespace: "ns-a", Version: 1},
		{Name: "one", Namespace: "ns-a", Version: 11},
		{Name: "one", Namespace: "ns-a", Version: 9},
		{Name: "one", Namespace: "ns-b", Version: 2},
		{Name: "two", Namespace: "ns-a", Version: 3},
	})

	assert.ElementsMatch(t, []driver.ReleaseMeta{
		{Name: "one", Namespace: "ns-a", Version: 11},
		{Name: "one", Namespace: "ns-b", Version: 2},
		{Name: "two", Namespace: "ns-a", Version: 3},
	}, metas, "the same release name in different namespaces must stay separate and the numeric max revision must win")
}

func TestAI_LastReleaseRevisionsEmpty(t *testing.T) {
	assert.Empty(t, lastReleaseRevisions(nil))
}

func TestAI_NewReleaseListResultRelease(t *testing.T) {
	deployedAt := time.Date(2026, time.September, 8, 10, 30, 0, 0, time.UTC)

	result := newReleaseListResultRelease(&helmrelease.Release{
		Name:      "one",
		Namespace: "ns-a",
		Version:   7,
		Info: &helmrelease.Info{
			Status:       helmrelease.StatusFailed,
			LastDeployed: helmtime.Time{Time: deployedAt},
			Annotations:  map[string]string{"key": "value"},
		},
		Chart: &chart.Chart{Metadata: &chart.Metadata{
			Name:       "mychart",
			Version:    "1.2.3",
			AppVersion: "4.5.6",
		}},
	})

	assert.Equal(t, "one", result.Name)
	assert.Equal(t, "ns-a", result.Namespace)
	assert.Equal(t, 7, result.Revision)
	assert.Equal(t, helmrelease.StatusFailed, result.Status)
	assert.Equal(t, map[string]string{"key": "value"}, result.Annotations)
	require.NotNil(t, result.DeployedAt)
	assert.Equal(t, int(deployedAt.Unix()), result.DeployedAt.Unix, "deploy time must come from the release info, not from a zero time")
	assert.Equal(t, deployedAt.String(), result.DeployedAt.Human)
	require.NotNil(t, result.Chart)
	assert.Equal(t, "mychart", result.Chart.Name)
	assert.Equal(t, "1.2.3", result.Chart.Version)
	assert.Equal(t, "4.5.6", result.Chart.AppVersion)
}

func TestAI_NewReleaseListResultReleaseWithoutInfoAndChart(t *testing.T) {
	result := newReleaseListResultRelease(&helmrelease.Release{Name: "one", Namespace: "ns-a", Version: 1})

	assert.Nil(t, result.DeployedAt)
	assert.Nil(t, result.Chart)
	assert.Empty(t, result.Status)

	result = newReleaseListResultRelease(&helmrelease.Release{Name: "one", Chart: &chart.Chart{}})

	assert.Nil(t, result.Chart, "a chart without metadata must not be reported")
}
