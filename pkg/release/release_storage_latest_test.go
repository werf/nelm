package release

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseStorage_AnnotationsSurviveStorageAndListing(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindSecret, "ns", 0)
	rls := newTestRelease("ns", "myrel", 1, helmreleasecommon.StatusDeployed)
	rls.Info.Annotations = map[string]string{"packages.deckhouse.io/managed-by": "deckhouse"}
	acc, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, s.storage.Create(ctx, acc))

	loaded, err := s.storage.GetRelease(ctx, "myrel", 1)
	require.NoError(t, err)
	assert.Equal(t, rls.Info.Annotations, loaded.Annotations())

	summaries, err := s.storage.ListLatestSummaries(ctx, ListLatestSummariesOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.NoError(t, summaries[0].DecodeErr)
	assert.Equal(t, rls.Info.Annotations, summaries[0].Summary.Annotations)
}

func TestReleaseStorage_LatestRevisionsSelectorBeforeMaximum(t *testing.T) {
	old := newTestStoredObject(t, newTestRelease("ns", "myrel", 2, helmreleasecommon.StatusSuperseded))
	old.Labels["packageChecksum"] = "old"
	newer := newTestStoredObject(t, newTestRelease("ns", "myrel", 3, helmreleasecommon.StatusDeployed))
	newer.Labels["packageChecksum"] = "new"
	other := newTestStoredObject(t, newTestRelease("a-ns", "other", 1, helmreleasecommon.StatusDeployed))
	other.Labels["packageChecksum"] = "old"
	backend := &latestListingBackend{objects: []*storedObject{newer, old, other}}

	revisions, err := newReleaseStorage("", backend, 0).LatestRevisions(context.Background(), LatestRevisionsOptions{LabelSelector: "packageChecksum=old"})
	require.NoError(t, err)
	assert.Equal(t, []Revision{
		{Name: "other", Namespace: "a-ns", Status: "deployed", Version: 1},
		{Name: "myrel", Namespace: "ns", Status: "superseded", Version: 2},
	}, revisions)
	assert.False(t, backend.withBodies)
	assert.Equal(t, "packageChecksum=old", backend.selector.String())

	_, err = newReleaseStorage("", backend, 0).LatestRevisions(context.Background(), LatestRevisionsOptions{LabelSelector: "team in ("})
	require.Error(t, err)
	assert.Equal(t, 1, backend.listings)
}

func TestReleaseStorage_ListLatestSummariesBackendError(t *testing.T) {
	backend := &latestListingBackend{err: errors.New("list error")}
	_, err := newReleaseStorage("", backend, 0).ListLatestSummaries(context.Background(), ListLatestSummariesOptions{})
	require.ErrorIs(t, err, backend.err)
}

func TestReleaseStorage_ListLatestSummariesDoesNotRetainBodies(t *testing.T) {
	ctx := logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard))
	backend := &latestListingBackend{}
	rls := newTestRelease("ns", "myrel", 1, helmreleasecommon.StatusDeployed)
	rls.Manifest = strings.Repeat("manifest content\n", 16384)

	bodies := make([][]byte, 3)
	for version := 1; version <= 3; version++ {
		rls.Version = version
		data, err := json.Marshal(rls)
		require.NoError(t, err)

		bodies[version-1] = []byte(base64.StdEncoding.EncodeToString(data))
	}

	var before, peak runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	backend.generate = func(fn func(*storedObject) error) error {
		for i := 0; i < 1000; i++ {
			for version := 1; version <= 3; version++ {
				obj := &storedObject{
					Namespace: "ns",
					Key:       fmt.Sprintf("sh.helm.release.v1.rel-%04d.v%d", i, version),
					Labels:    map[string]string{"owner": "helm", "name": fmt.Sprintf("rel-%04d", i), "version": strconv.Itoa(version), "status": "deployed"},
					Body:      append([]byte(nil), bodies[version-1]...),
				}
				if err := fn(obj); err != nil {
					return err
				}
			}

			if i == 998 {
				runtime.GC()
				runtime.ReadMemStats(&peak)
			}
		}

		return nil
	}

	summaries, err := newReleaseStorage("", backend, 0).ListLatestSummaries(ctx, ListLatestSummariesOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 1000)

	for _, summary := range summaries {
		require.NoError(t, summary.DecodeErr)
		assert.Equal(t, 3, summary.Revision.Version)
	}

	growth := uint64(0)
	if peak.HeapAlloc > before.HeapAlloc {
		growth = peak.HeapAlloc - before.HeapAlloc
	}

	t.Logf("synthetic streaming live heap growth: %.2f MiB", float64(growth)/(1<<20))
	assert.Less(t, growth, uint64(32<<20), "live heap must not retain all decoded manifests")
}

func TestReleaseStorage_ListLatestSummariesGroupsAndReplaces(t *testing.T) {
	objects := []*storedObject{
		newUndecodableTestStoredObject(t, "ns", "foo", 1, helmreleasecommon.StatusSuperseded),
		newTestStoredObject(t, newTestRelease("ns", "foo", 10, helmreleasecommon.StatusDeployed)),
		newTestStoredObject(t, newTestRelease("ns", "foo.v1a", 1, helmreleasecommon.StatusDeployed)),
		newTestStoredObject(t, newTestRelease("ns", "foo", 11, helmreleasecommon.StatusDeployed)),
		newUndecodableTestStoredObject(t, "ns", "foo", 9, helmreleasecommon.StatusFailed),
		newTestStoredObject(t, newTestRelease("other", "foo", 3, helmreleasecommon.StatusDeployed)),
		newTestStoredObject(t, newTestRelease("ns", "foo", 5, helmreleasecommon.StatusSuperseded)),
		newUndecodableTestStoredObject(t, "ns", "broken", 1, helmreleasecommon.StatusDeployed),
	}
	backend := &latestListingBackend{objects: objects}

	summaries, err := newReleaseStorage("", backend, 0).ListLatestSummaries(context.Background(), ListLatestSummariesOptions{})
	require.NoError(t, err)

	var events []string
	for _, summary := range summaries {
		if summary.Revision.Name == "broken" {
			require.ErrorIs(t, summary.DecodeErr, ErrReleaseUndecodable)
			assert.Nil(t, summary.Summary)
		} else {
			require.NoError(t, summary.DecodeErr)
			require.NotNil(t, summary.Summary)
		}

		events = append(events, fmt.Sprintf("%s/%s/%d", summary.Revision.Namespace, summary.Revision.Name, summary.Revision.Version))
	}

	assert.Equal(t, []string{"ns/broken/1", "ns/foo/11", "ns/foo.v1a/1", "other/foo/3"}, events)
	assert.Equal(t, 1, backend.listings)
}

func TestReleaseStorage_ListLatestSummariesInvalidSelectorMakesNoRequests(t *testing.T) {
	backend := &latestListingBackend{}
	_, err := newReleaseStorage("", backend, 0).ListLatestSummaries(context.Background(), ListLatestSummariesOptions{LabelSelector: "team in ("})
	require.Error(t, err)
	assert.Zero(t, backend.listings)
}

func TestReleaseStorage_ListLatestSummariesMissingInfo(t *testing.T) {
	obj := &storedObject{
		Namespace: "ns", Key: storageKey("myrel", 1),
		Labels: map[string]string{"owner": "helm", "name": "myrel", "version": "1"},
		Body:   []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"myrel","namespace":"ns","version":1}`))),
	}
	backend := &latestListingBackend{objects: []*storedObject{obj}}
	summaries, err := newReleaseStorage("", backend, 0).ListLatestSummaries(context.Background(), ListLatestSummariesOptions{})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.ErrorIs(t, summaries[0].DecodeErr, ErrReleaseUndecodable)
	assert.Nil(t, summaries[0].Summary)
}

func TestReleaseStorage_ListLatestSummariesSelectorBeforeMaximum(t *testing.T) {
	old := newTestStoredObject(t, newTestRelease("ns", "myrel", 2, helmreleasecommon.StatusSuperseded))
	old.Labels["packageChecksum"] = "old"
	newer := newTestStoredObject(t, newTestRelease("ns", "myrel", 3, helmreleasecommon.StatusDeployed))
	newer.Labels["packageChecksum"] = "new"
	backend := &latestListingBackend{objects: []*storedObject{old, newer}}

	summaries, err := newReleaseStorage("ns", backend, 0).ListLatestSummaries(context.Background(), ListLatestSummariesOptions{LabelSelector: "packageChecksum=old"})
	require.NoError(t, err)
	assert.Equal(t, []int{2}, summaryVersions(t, summaries))
	assert.Equal(t, "packageChecksum=old", backend.selector.String())
}

func TestReleaseStorage_LoadRevisionSummary(t *testing.T) {
	ctx := context.Background()
	s := newTestKubeStorage(t, kubeStorageKindSecret, "ns", 0)
	rls := newTestRelease("ns", "myrel", 1, helmreleasecommon.StatusDeployed)
	rls.Info.Annotations = map[string]string{"managed-by": "deckhouse"}
	acc, err := helmrel.NewAccessor(rls)
	require.NoError(t, err)
	require.NoError(t, s.storage.Create(ctx, acc))

	cluster := newReleaseStorage("", s.backend, 0)

	summary, err := cluster.LoadRevisionSummary(ctx, Revision{Name: "myrel", Namespace: "ns", Version: 1})
	require.NoError(t, err)
	assert.Equal(t, rls.Info.Annotations, summary.Annotations)

	_, err = cluster.LoadRevisionSummary(ctx, Revision{Name: "myrel", Namespace: "ns", Version: 2})
	require.ErrorIs(t, err, ErrReleaseNotFound)
	require.NotErrorIs(t, err, ErrReleaseUndecodable)

	_, err = cluster.LoadRevisionSummary(ctx, Revision{Name: "myrel", Version: 1})
	require.ErrorContains(t, err, "namespace is required")
}
