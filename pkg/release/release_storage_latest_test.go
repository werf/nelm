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

	var calls int
	require.NoError(t, s.storage.ForEachLatestRelease(ctx, func(_ Revision, rel helmrel.Accessor, err error) error {
		require.NoError(t, err)
		assert.Equal(t, rls.Info.Annotations, rel.Annotations())

		calls++

		return nil
	}, ForEachLatestReleaseOptions{}))
	assert.Equal(t, 1, calls)
}

func TestReleaseStorage_ForEachLatestReleaseCallbackAndBackendErrors(t *testing.T) {
	callbackErr := errors.New("callback error")
	backend := &latestListingBackend{objects: []*storedObject{newTestStoredObject(t, newTestRelease("ns", "myrel", 1, helmreleasecommon.StatusDeployed))}}
	err := newReleaseStorage("", backend, 0).ForEachLatestRelease(context.Background(), func(Revision, helmrel.Accessor, error) error { return callbackErr }, ForEachLatestReleaseOptions{})
	assert.Equal(t, callbackErr, err)

	backend.err = errors.New("list error")
	err = newReleaseStorage("", backend, 0).ForEachLatestRelease(context.Background(), func(Revision, helmrel.Accessor, error) error { return nil }, ForEachLatestReleaseOptions{})
	require.ErrorIs(t, err, backend.err)
}

func TestReleaseStorage_ForEachLatestReleaseDoesNotRetainBodies(t *testing.T) {
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

	var revisions []Revision
	require.NoError(t, newReleaseStorage("", backend, 0).ForEachLatestRelease(ctx, func(revision Revision, rel helmrel.Accessor, err error) error {
		require.NoError(t, err)
		assert.Equal(t, 3, revision.Version)
		assert.Equal(t, 3, rel.Version())

		revisions = append(revisions, revision)

		return nil
	}, ForEachLatestReleaseOptions{}))
	require.Len(t, revisions, 1000)

	growth := uint64(0)
	if peak.HeapAlloc > before.HeapAlloc {
		growth = peak.HeapAlloc - before.HeapAlloc
	}

	t.Logf("synthetic streaming live heap growth: %.2f MiB", float64(growth)/(1<<20))
	assert.Less(t, growth, uint64(32<<20), "live heap must not retain all decoded manifests")
}

func TestReleaseStorage_ForEachLatestReleaseGroupsAndReplaces(t *testing.T) {
	objects := []*storedObject{
		newUndecodableTestStoredObject(t, "ns", "foo", 1, helmreleasecommon.StatusSuperseded),
		newTestStoredObject(t, newTestRelease("ns", "foo", 10, helmreleasecommon.StatusDeployed)),
		newTestStoredObject(t, newTestRelease("ns", "foo.v1a", 1, helmreleasecommon.StatusDeployed)),
		newTestStoredObject(t, newTestRelease("ns", "foo", 11, helmreleasecommon.StatusDeployed)),
		newUndecodableTestStoredObject(t, "ns", "foo", 9, helmreleasecommon.StatusFailed),
		newTestStoredObject(t, newTestRelease("other", "foo", 3, helmreleasecommon.StatusDeployed)),
		newUndecodableTestStoredObject(t, "ns", "broken", 1, helmreleasecommon.StatusDeployed),
	}
	backend := &latestListingBackend{objects: objects}

	var events []string

	err := newReleaseStorage("", backend, 0).ForEachLatestRelease(context.Background(), func(revision Revision, rel helmrel.Accessor, err error) error {
		if revision.Name == "broken" {
			require.ErrorIs(t, err, ErrReleaseUndecodable)
			assert.Nil(t, rel)
		} else {
			require.NoError(t, err)
			assert.Equal(t, revision.Version, rel.Version())
		}

		events = append(events, fmt.Sprintf("%s/%s/%d", revision.Namespace, revision.Name, revision.Version))

		return nil
	}, ForEachLatestReleaseOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"ns/foo/10", "ns/foo.v1a/1", "ns/foo/11", "other/foo/3", "ns/broken/1"}, events)
	assert.Equal(t, 1, backend.listings)
}

func TestReleaseStorage_ForEachLatestReleaseInvalidSelectorMakesNoRequests(t *testing.T) {
	backend := &latestListingBackend{}
	err := newReleaseStorage("", backend, 0).ForEachLatestRelease(context.Background(), func(Revision, helmrel.Accessor, error) error {
		t.Fatal("unexpected callback")

		return nil
	}, ForEachLatestReleaseOptions{LabelSelector: "team in ("})
	require.Error(t, err)
	assert.Zero(t, backend.listings)
}

func TestReleaseStorage_ForEachLatestReleaseMissingInfo(t *testing.T) {
	obj := &storedObject{
		Namespace: "ns", Key: storageKey("myrel", 1),
		Labels: map[string]string{"owner": "helm", "name": "myrel", "version": "1"},
		Body:   []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"myrel","namespace":"ns","version":1}`))),
	}
	backend := &latestListingBackend{objects: []*storedObject{obj}}
	err := newReleaseStorage("", backend, 0).ForEachLatestRelease(context.Background(), func(_ Revision, rel helmrel.Accessor, err error) error {
		require.ErrorIs(t, err, ErrReleaseUndecodable)
		assert.Nil(t, rel)

		return nil
	}, ForEachLatestReleaseOptions{})
	require.NoError(t, err)
}

func TestReleaseStorage_ForEachLatestReleaseSelectorBeforeMaximum(t *testing.T) {
	old := newTestStoredObject(t, newTestRelease("ns", "myrel", 2, helmreleasecommon.StatusSuperseded))
	old.Labels["packageChecksum"] = "old"
	newer := newTestStoredObject(t, newTestRelease("ns", "myrel", 3, helmreleasecommon.StatusDeployed))
	newer.Labels["packageChecksum"] = "new"
	backend := &latestListingBackend{objects: []*storedObject{old, newer}}

	var versions []int

	err := newReleaseStorage("ns", backend, 0).ForEachLatestRelease(context.Background(), func(revision Revision, _ helmrel.Accessor, err error) error {
		require.NoError(t, err)

		versions = append(versions, revision.Version)

		return nil
	}, ForEachLatestReleaseOptions{LabelSelector: "packageChecksum=old"})
	require.NoError(t, err)
	assert.Equal(t, []int{2}, versions)
	assert.Equal(t, "packageChecksum=old", backend.selector.String())
}
