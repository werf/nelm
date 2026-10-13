package release

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseStorage_KubeAndMemoryAnnotationsReplaceAndPreserve(t *testing.T) {
	ctx := context.Background()
	kubeSecret := newTestKubeStorage(t, kubeStorageKindSecret, "ns", 0)

	kubeConfigMap := newTestKubeStorage(t, kubeStorageKindConfigMap, "ns", 0)
	for name, backend := range map[string]storageBackend{"secret": kubeSecret.backend, "configmap": kubeConfigMap.backend, "memory": newMemoryStorageBackend()} {
		t.Run(name, func(t *testing.T) {
			storage := newReleaseStorage("ns", backend, 0)
			rls := newTestRelease("ns", "app", 1, common.StatusPendingInstall)
			rls.Info.Annotations = map[string]string{"team": "old", "remove": "yes"}
			accessor, err := helmrel.NewAccessor(rls)
			require.NoError(t, err)
			require.NoError(t, storage.Create(ctx, accessor))
			stored, err := backend.get(ctx, "ns", storageKey("app", 1))
			require.NoError(t, err)
			assert.Equal(t, "old", stored.Annotations["team"])
			assert.Equal(t, storageAnnotationReleasedByValue, stored.Annotations[storageAnnotationReleasedBy])

			accessor, err = storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			accessor.SetStatus(common.StatusDeployed)
			require.NoError(t, storage.Update(ctx, accessor))
			accessor, err = storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			assert.Equal(t, "old", accessor.Annotations()["team"])
			require.NoError(t, storage.UpdateLabels(ctx, "app", 1, map[string]string{"team": "label"}))
			stored, err = backend.get(ctx, "ns", storageKey("app", 1))
			require.NoError(t, err)
			assert.Equal(t, "old", stored.Annotations["team"])

			updated, err := ReleaserToV1Release(accessor.Releaser())
			require.NoError(t, err)

			updated.Info.Annotations = map[string]string{"team": "new"}

			require.NoError(t, storage.Update(ctx, accessor))
			accessor, err = storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			assert.Equal(t, "new", accessor.Annotations()["team"])
			assert.NotContains(t, accessor.Annotations(), "remove")

			updated, err = ReleaserToV1Release(accessor.Releaser())
			require.NoError(t, err)

			updated.Info.Annotations = nil

			require.NoError(t, storage.Update(ctx, accessor))
			accessor, err = storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{storageAnnotationReleasedBy: storageAnnotationReleasedByValue}, accessor.Annotations())
		})
	}
}

func TestReleaseStorage_KubeMetadataIncludesAnnotations(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			s := newTestKubeStorage(t, kind, "ns", 0)
			rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
			rls.Info.Annotations = map[string]string{"team": "platform"}
			obj := newTestStoredObject(t, rls)
			putTestKubeObject(t, s, obj)
			s.client.ClearActions()
			revisions, err := s.storage.LatestRevisions(context.Background(), LatestRevisionsOptions{})
			require.NoError(t, err)
			require.Len(t, revisions, 1)
			assert.Equal(t, obj.Annotations, revisions[0].Annotations)
			assert.Empty(t, s.client.Actions())
		})
	}
}

func TestReleaseStorage_KubeValidatesBeforePruningOrUpdating(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			s := newTestKubeStorage(t, kind, "ns", 1)
			previous := newTestRelease("ns", "app", 1, common.StatusFailed)
			putTestKubeObject(t, s, newTestStoredObject(t, previous))

			for name, annotations := range map[string]map[string]string{
				"invalid key": {"bad key": "value"},
				"oversized":   {"value": strings.Repeat("x", 256*1024)},
				"reserved":    {storageAnnotationReleasedBy: "user"},
			} {
				t.Run(name, func(t *testing.T) {
					next := newTestRelease("ns", "app", 2, common.StatusPendingUpgrade)
					next.Info.Annotations = annotations
					accessor, err := helmrel.NewAccessor(next)
					require.NoError(t, err)
					s.client.ClearActions()
					require.Error(t, s.storage.Create(context.Background(), accessor))
					assert.Empty(t, s.client.Actions())
					_, err = s.backend.get(context.Background(), "ns", storageKey("app", 1))
					require.NoError(t, err)
					s.client.ClearActions()
					require.Error(t, s.storage.Update(context.Background(), accessor))
					assert.Empty(t, s.client.Actions())
				})
			}
		})
	}
}

func TestStoredObject_AnnotationsNotEncodedInBody(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "platform"}

	obj, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.NoError(t, err)

	body, err := decodeRelease(obj.Body)
	require.NoError(t, err)
	assert.Empty(t, body.Info.Annotations)
	assert.Equal(t, map[string]string{"team": "platform"}, rls.Info.Annotations)
}

func TestStoredObject_EmptyAnnotationsHaveMarker(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	obj, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{storageAnnotationReleasedBy: storageAnnotationReleasedByValue}, obj.Annotations)
	assert.Nil(t, rls.Info.Annotations)

	loaded, err := decodeStoredObject(obj)
	require.NoError(t, err)
	assert.Equal(t, obj.Annotations, loaded.Info.Annotations)
}

func TestStoredObject_MetadataAndLegacyAnnotations(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "legacy"}
	legacy := &storedObject{Namespace: "ns", Key: storageKey("app", 1), Body: encodeHelmRelease(t, rls)}
	loaded, err := decodeStoredObject(legacy)
	require.NoError(t, err)
	assert.Equal(t, rls.Info.Annotations, loaded.Info.Annotations)

	summary, err := decodeStoredSummary(legacy)
	require.NoError(t, err)
	assert.Equal(t, rls.Info.Annotations, summary.Annotations)

	legacy.Annotations = map[string]string{storageAnnotationReleasedBy: storageAnnotationReleasedByValue, "team": "metadata"}
	loaded, err = decodeStoredObject(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy.Annotations, loaded.Info.Annotations)
	summary, err = decodeStoredSummary(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy.Annotations, summary.Annotations)
	loaded.Info.Annotations["team"] = "changed"
	assert.Equal(t, "metadata", legacy.Annotations["team"])
}

func TestStoredObject_ReservedWriterCannotBeChanged(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{storageAnnotationReleasedBy: "user"}
	_, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.ErrorContains(t, err, "reserved")
}
