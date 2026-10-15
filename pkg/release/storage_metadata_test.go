package release

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseStorage_LatestRevisionsSkipAnnotationsKeepsStoredMetadata(t *testing.T) {
	for name, backend := range map[string]func(t *testing.T, obj *storedObject) storageBackend{
		"secret": func(t *testing.T, obj *storedObject) storageBackend {
			s := newTestKubeStorage(t, kubeStorageKindSecret, "ns", 0)
			putTestKubeObject(t, s, obj)

			return s.backend
		},
		"configmap": func(t *testing.T, obj *storedObject) storageBackend {
			s := newTestKubeStorage(t, kubeStorageKindConfigMap, "ns", 0)
			putTestKubeObject(t, s, obj)

			return s.backend
		},
		"memory": func(t *testing.T, obj *storedObject) storageBackend {
			b := newMemoryStorageBackend()
			require.NoError(t, b.create(context.Background(), obj))

			return b
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
			rls.Info.Annotations = map[string]string{"team": "platform"}
			obj := newTestStoredObject(t, rls)
			b := backend(t, obj)
			storage := newReleaseStorage("ns", b, 0)
			revisions, err := storage.LatestRevisions(ctx, LatestRevisionsOptions{SkipAnnotations: true})
			require.NoError(t, err)
			require.Len(t, revisions, 1)
			assert.Nil(t, revisions[0].Annotations)
			revisions, err = storage.LatestRevisions(ctx, LatestRevisionsOptions{})
			require.NoError(t, err)
			require.Len(t, revisions, 1)
			assert.Equal(t, obj.Annotations, revisions[0].Annotations)
			stored, err := b.get(ctx, "ns", obj.Key)
			require.NoError(t, err)
			assert.Equal(t, obj.Annotations, stored.Annotations)
		})
	}
}

func TestReleaseStorage_PruningStillRejectsInvalidVersions(t *testing.T) {
	ctx := context.Background()
	backend := newMemoryStorageBackend()
	obj := newTestStoredObject(t, newTestRelease(testNamespace, "app", 1, common.StatusDeployed))
	obj.Labels[storageLabelVersion] = "garbage"
	require.NoError(t, backend.create(ctx, obj))
	storage := newReleaseStorage(testNamespace, backend, 0)
	require.Error(t, storage.removeOldestRevisions(ctx, "app", 0))
	_, err := backend.get(ctx, testNamespace, obj.Key)
	require.NoError(t, err)
}

func TestReleaseStorage_VersionReadsDoNotKeepAnnotations(t *testing.T) {
	ctx := context.Background()
	rls := newTestReleaseWithStatus("app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "platform"}
	storage := newMemoryReleaseStorage(t, rls)

	revisions, err := storage.readableRevisions(ctx, testNamespace, "app")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	assert.Nil(t, revisions[0].Annotations)

	revisions, err = storage.Revisions(ctx, "app")
	require.NoError(t, err)
	require.Len(t, revisions, 1)
	assert.Equal(t, "platform", revisions[0].Annotations["team"])
	assert.Equal(t, storageAnnotationReleaseVersionV1, revisions[0].Annotations[storageAnnotationReleaseVersion])

	loaded, err := storage.GetRelease(ctx, "app", 0)
	require.NoError(t, err)
	assert.Equal(t, revisions[0].Annotations, loaded.Annotations())
}
