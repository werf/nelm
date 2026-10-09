package action

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
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

func TestBuildReleaseHistoryResult(t *testing.T) {
	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	withoutChart := newLatestRevisionSummary("ns", "myrel", 2)
	withoutChart.Summary.Chart = nil
	storage := &latestReleaseListStorager{historySummaries: []release.RevisionSummary{
		newLatestRevisionSummary("ns", "myrel", 1),
		withoutChart,
		{Revision: release.Revision{Namespace: "ns", Name: "myrel", Version: 3, Status: "failed"}, DecodeErr: release.ErrReleaseUndecodable},
	}}

	result, err := buildReleaseHistoryResult(ctx, storage, "myrel", 5)
	require.NoError(t, err)
	assert.Equal(t, "myrel", storage.historyName)
	assert.Equal(t, 5, storage.historyLimit)
	require.Len(t, result.Releases, 3)

	assert.Equal(t, &ReleaseHistoryResultChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"}, result.Releases[0].Chart)
	assert.Equal(t, map[string]string{"managed-by": "deckhouse"}, result.Releases[0].Annotations)
	require.NotNil(t, result.Releases[0].DeployedAt)

	assert.Nil(t, result.Releases[1].Chart)
	require.NotNil(t, result.Releases[1].DeployedAt)

	assert.Equal(t, &ReleaseHistoryResultRelease{Name: "myrel", Namespace: "ns", Revision: 3, Status: helmreleasestatus.StatusFailed}, result.Releases[2])
	require.NotPanics(t, func() { buildReleaseHistoryOutputTable(ctx, result).Render() })

	_, err = buildReleaseHistoryResult(ctx, &latestReleaseListStorager{err: errors.New("connection refused")}, "myrel", 0)
	require.ErrorContains(t, err, "connection refused")
}

func TestBuildReleaseListResultBackendError(t *testing.T) {
	backendErr := errors.New("connection refused")
	_, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{err: backendErr}, "")
	require.ErrorIs(t, err, backendErr)

	_, err = buildReleaseListResultFromMetadata(context.Background(), &latestReleaseListStorager{err: backendErr}, "")
	require.ErrorIs(t, err, backendErr)
}

func TestBuildReleaseListResultEmpty(t *testing.T) {
	result, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{}, "")
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiVersion":"v2","releases":[]}`, string(data))
}

func TestBuildReleaseListResultFromMetadataCachesUndecodable(t *testing.T) {
	ctx := context.Background()
	storage := newMetadataReleaseListStorager([]release.Revision{{Namespace: "ns", Name: "a", Version: 1}}, nil, fmt.Errorf("decode: %w", release.ErrReleaseUndecodable))

	result, err := buildReleaseListResultFromMetadata(ctx, storage, "")
	require.NoError(t, err)

	for range 2 {
		_, err = result.Releases[0].Chart(ctx)
		require.ErrorIs(t, err, release.ErrReleaseUndecodable)
	}

	assert.Len(t, storage.loadedRevisions(), 1)
}

func TestBuildReleaseListResultFromMetadataLoadsOnce(t *testing.T) {
	ctx := context.Background()
	storage := newMetadataReleaseListStorager([]release.Revision{
		{Namespace: "ns", Name: "a", Version: 2, Status: "deployed"},
		{Namespace: "ns", Name: "b", Version: 1, Status: "failed"},
	}, map[string]*release.ReleaseSummary{"ns/a": newTestReleaseSummary(), "ns/b": newTestReleaseSummary()})

	result, err := buildReleaseListResultFromMetadata(ctx, storage, "packageChecksum")
	require.NoError(t, err)
	assert.Equal(t, "packageChecksum", storage.selector)
	assert.Empty(t, storage.loadedRevisions())

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiVersion":"v2","releases":[{"name":"a","namespace":"ns","revision":2,"status":"deployed","deployedAt":null,"annotations":null,"chart":null},{"name":"b","namespace":"ns","revision":1,"status":"failed","deployedAt":null,"annotations":null,"chart":null}]}`, string(data))
	assert.Empty(t, storage.loadedRevisions())

	rel := result.Releases[0]

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			annotations, err := rel.Annotations(ctx)
			assert.NoError(t, err)
			assert.Equal(t, map[string]string{"managed-by": "deckhouse"}, annotations)
		}()
	}

	wg.Wait()

	_, err = rel.Chart(ctx)
	require.NoError(t, err)
	_, err = rel.DeployedAt(ctx)
	require.NoError(t, err)
	assert.Equal(t, []release.Revision{{Namespace: "ns", Name: "a", Version: 2, Status: "deployed"}}, storage.loadedRevisions())

	annotations, err := rel.Annotations(ctx)
	require.NoError(t, err)

	annotations["managed-by"] = "changed"

	data, err = json.Marshal(rel)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"managed-by":"deckhouse"`)
}

func TestBuildReleaseListResultFromMetadataRetriesTransientErrors(t *testing.T) {
	ctx := context.Background()
	transient := errors.New("connection refused")
	storage := newMetadataReleaseListStorager([]release.Revision{{Namespace: "ns", Name: "a", Version: 1}}, map[string]*release.ReleaseSummary{"ns/a": newTestReleaseSummary()}, transient, release.ErrReleaseNotFound)

	result, err := buildReleaseListResultFromMetadata(ctx, storage, "")
	require.NoError(t, err)

	rel := result.Releases[0]

	_, err = rel.Annotations(ctx)
	require.ErrorIs(t, err, transient)
	_, err = rel.Annotations(ctx)
	require.ErrorIs(t, err, release.ErrReleaseNotFound)
	_, err = rel.Annotations(ctx)
	require.NoError(t, err)
	_, err = rel.Annotations(ctx)
	require.NoError(t, err)
	assert.Len(t, storage.loadedRevisions(), 3)
}

func TestBuildReleaseListResultProjectsAndSorts(t *testing.T) {
	ctx := context.Background()
	storage := &latestReleaseListStorager{latestSummaries: []release.RevisionSummary{
		newLatestRevisionSummary("z", "same", 3),
		newLatestRevisionSummary("a", "same", 10),
		newLatestRevisionSummary("a", "other", 1),
	}}

	result, err := buildReleaseListResult(ctx, storage, "packageChecksum")
	require.NoError(t, err)
	assert.Equal(t, "packageChecksum", storage.selector)
	assert.Equal(t, "v2", result.APIVersion)
	require.Len(t, result.Releases, 3)
	assert.Equal(t, []string{"a/other/1", "a/same/10", "z/same/3"}, lo.Map(result.Releases, func(rel *ReleaseListResultRelease, _ int) string {
		return fmt.Sprintf("%s/%s/%d", rel.Namespace, rel.Name, rel.Revision)
	}))

	for _, rel := range result.Releases {
		annotations, err := rel.Annotations(ctx)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"managed-by": "deckhouse"}, annotations)

		chart, err := rel.Chart(ctx)
		require.NoError(t, err)
		assert.Equal(t, &ReleaseListResultChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"}, chart)

		deployedAt, err := rel.DeployedAt(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, deployedAt.Unix-int(time.Time{}.Unix()))
	}

	assert.Empty(t, storage.loadedRevisions())
}

func TestBuildReleaseListResultUndecodableLatest(t *testing.T) {
	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	storage := &latestReleaseListStorager{latestSummaries: []release.RevisionSummary{
		{Revision: release.Revision{Namespace: "ns", Name: "myrel", Version: 2, Status: "failed"}, DecodeErr: release.ErrReleaseUndecodable},
	}}

	result, err := buildReleaseListResult(ctx, storage, "")
	require.NoError(t, err)
	require.Len(t, result.Releases, 1)

	rel := result.Releases[0]
	assert.Equal(t, 2, rel.Revision)
	assert.Equal(t, helmreleasestatus.StatusFailed, rel.Status)

	_, err = rel.Annotations(ctx)
	require.ErrorIs(t, err, release.ErrReleaseUndecodable)
	_, err = rel.Chart(ctx)
	require.ErrorIs(t, err, release.ErrReleaseUndecodable)
	_, err = rel.DeployedAt(ctx)
	require.ErrorIs(t, err, release.ErrReleaseUndecodable)

	data, err := json.Marshal(rel)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"myrel","namespace":"ns","revision":2,"status":"failed","deployedAt":null,"annotations":null,"chart":null}`, string(data))
}

func TestListsReleasesByMetadata(t *testing.T) {
	for _, tt := range []struct {
		opts     ReleaseListOptions
		expected bool
	}{
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatTable}, expected: true},
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatJSON}, expected: false},
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatYAML}, expected: false},
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatTable, OutputNoPrint: true}, expected: false},
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatJSON, MetadataOnly: true}, expected: true},
		{opts: ReleaseListOptions{OutputFormat: common.OutputFormatTable, OutputNoPrint: true, MetadataOnly: true}, expected: true},
	} {
		assert.Equal(t, tt.expected, listsReleasesByMetadata(tt.opts), "%+v", tt.opts)
	}
}

func TestReleaseListResultMarshalsEagerDetails(t *testing.T) {
	entry := newLatestRevisionSummary("ns", "myrel", 1)
	entry.Summary.DeployedAt = time.Unix(1735689600, 0).UTC()

	result, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{latestSummaries: []release.RevisionSummary{entry}}, "")
	require.NoError(t, err)

	data, err := json.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"apiVersion":"v2","releases":[{"name":"myrel","namespace":"ns","revision":1,"status":"deployed","deployedAt":{"human":"2025-01-01 00:00:00 +0000 UTC","unix":1735689600},"annotations":{"managed-by":"deckhouse"},"chart":{"name":"mychart","version":"1.2.3","appVersion":"4.5.6"}}]}`, string(data))

	yamlData, err := yaml.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "managed-by: deckhouse")
	assert.Contains(t, string(yamlData), "appVersion: 4.5.6")
	assert.Contains(t, string(yamlData), "apiVersion: v2")
}

func TestReleaseListResultReleaseCancelledReaderLetsWaiterRead(t *testing.T) {
	storage := newMetadataReleaseListStorager([]release.Revision{{Namespace: "ns", Name: "a", Version: 1}}, map[string]*release.ReleaseSummary{"ns/a": newTestReleaseSummary()})
	storage.loadGate, storage.loadStarted = make(chan struct{}), make(chan struct{}, 10)

	result, err := buildReleaseListResultFromMetadata(context.Background(), storage, "")
	require.NoError(t, err)

	rel := result.Releases[0]

	readerCtx, cancelReader := context.WithCancel(context.Background())

	readerDone := make(chan error, 1)
	go func() {
		_, err := rel.Chart(readerCtx)
		readerDone <- err
	}()

	<-storage.loadStarted

	waiterDone := make(chan error, 1)
	go func() {
		_, err := rel.Annotations(context.Background())
		waiterDone <- err
	}()

	cancelReader()
	require.ErrorIs(t, <-readerDone, context.Canceled)

	<-storage.loadStarted
	close(storage.loadGate)
	require.NoError(t, <-waiterDone)
}

func TestReleaseListResultReleaseMarshalsByValue(t *testing.T) {
	entry := newLatestRevisionSummary("ns", "myrel", 1)
	result, err := buildReleaseListResult(context.Background(), &latestReleaseListStorager{latestSummaries: []release.RevisionSummary{entry}}, "")
	require.NoError(t, err)

	byPointer, err := json.Marshal(result.Releases[0])
	require.NoError(t, err)

	byValue, err := json.Marshal(*result.Releases[0])
	require.NoError(t, err)
	assert.JSONEq(t, string(byPointer), string(byValue))
	assert.Contains(t, string(byValue), `"managed-by":"deckhouse"`)

	yamlByValue, err := yaml.Marshal(*result.Releases[0])
	require.NoError(t, err)
	assert.Contains(t, string(yamlByValue), "managed-by: deckhouse")
}

func TestReleaseListResultReleaseNotFromList(t *testing.T) {
	rel := &ReleaseListResultRelease{Name: "a", Namespace: "ns"}

	_, err := rel.Annotations(context.Background())
	require.ErrorContains(t, err, "not returned by ReleaseList")

	data, err := json.Marshal(rel)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"a","namespace":"ns","revision":0,"status":"","deployedAt":null,"annotations":null,"chart":null}`, string(data))
}

func TestReleaseListResultReleaseWaiterHonoursItsContext(t *testing.T) {
	storage := newMetadataReleaseListStorager([]release.Revision{{Namespace: "ns", Name: "a", Version: 1}}, map[string]*release.ReleaseSummary{"ns/a": newTestReleaseSummary()})
	storage.loadGate, storage.loadStarted = make(chan struct{}), make(chan struct{}, 10)

	result, err := buildReleaseListResultFromMetadata(context.Background(), storage, "")
	require.NoError(t, err)

	rel := result.Releases[0]

	loaderDone := make(chan error, 1)
	go func() {
		_, err := rel.Chart(context.Background())
		loaderDone <- err
	}()

	<-storage.loadStarted

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err = rel.Annotations(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), time.Second)

	data, err := json.Marshal(rel)
	require.NoError(t, err, "marshaling must not wait for the running read")
	assert.Contains(t, string(data), `"chart":null`)

	close(storage.loadGate)
	require.NoError(t, <-loaderDone)

	annotations, err := rel.Annotations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"managed-by": "deckhouse"}, annotations)
	assert.Len(t, storage.loadedRevisions(), 1)
}

func TestReleaseListResultReleaseWithoutChart(t *testing.T) {
	ctx := context.Background()
	entry := newLatestRevisionSummary("ns", "myrel", 1)
	entry.Summary.Chart = nil

	result, err := buildReleaseListResult(ctx, &latestReleaseListStorager{latestSummaries: []release.RevisionSummary{entry}}, "")
	require.NoError(t, err)

	_, err = result.Releases[0].Chart(ctx)
	require.ErrorContains(t, err, "no chart metadata")

	annotations, err := result.Releases[0].Annotations(ctx)
	require.NoError(t, err)
	assert.Equal(t, entry.Summary.Annotations, annotations)
}
