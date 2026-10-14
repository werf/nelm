package release

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2release "github.com/werf/nelm/v2/pkg/helm/intern/release/v2"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	common "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	v1release "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/v2/pkg/helm/pkg/storage/driver"
)

func TestReleaseStorage_KubeAndMemoryAnnotationsReplaceAndPreserve(t *testing.T) {
	for _, releaseCase := range []struct {
		name       string
		newRelease func(map[string]string) helmrel.Releaser
	}{
		{
			name: "v1",
			newRelease: func(annotations map[string]string) helmrel.Releaser {
				rls := newTestRelease("ns", "app", 1, common.StatusPendingInstall)
				rls.Info.Annotations = annotations

				return rls
			},
		},
		{
			name: "v2",
			newRelease: func(annotations map[string]string) helmrel.Releaser {
				return &v2release.Release{
					Name:      "app",
					Namespace: "ns",
					Version:   1,
					Info:      &v2release.Info{Status: common.StatusPendingInstall, Annotations: annotations},
				}
			},
		},
	} {
		t.Run(releaseCase.name, func(t *testing.T) {
			for name, newBackend := range map[string]func(*testing.T) storageBackend{
				"secret": func(t *testing.T) storageBackend {
					return newTestKubeStorage(t, kubeStorageKindSecret, "ns", 0).backend
				},
				"configmap": func(t *testing.T) storageBackend {
					return newTestKubeStorage(t, kubeStorageKindConfigMap, "ns", 0).backend
				},
				"memory": func(t *testing.T) storageBackend {
					return newMemoryStorageBackend()
				},
			} {
				t.Run(name, func(t *testing.T) {
					ctx := context.Background()
					backend := newBackend(t)
					storage := newReleaseStorage("ns", backend, 0)
					rls := releaseCase.newRelease(map[string]string{"team": "old", "remove": "yes"})
					inputAccessor, err := helmrel.NewAccessor(rls)
					require.NoError(t, err)
					require.NoError(t, storage.Create(ctx, inputAccessor))
					assert.Equal(t, map[string]string{"team": "old", "remove": "yes"}, inputAccessor.Annotations())

					stored, err := backend.get(ctx, "ns", storageKey("app", 1))
					require.NoError(t, err)

					originalAnnotations := map[string]string{
						"team":                         "old",
						"remove":                       "yes",
						storageAnnotationReleaseFormat: storageAnnotationReleaseFormatV1,
					}
					assert.Equal(t, originalAnnotations, stored.Annotations)
					body, err := decodeRelease(stored.Body)
					require.NoError(t, err)
					assert.Equal(t, stored.Annotations, body.Info.Annotations)

					accessor, err := storage.GetRelease(ctx, "app", 1)
					require.NoError(t, err)
					assert.IsType(t, &v1release.Release{}, accessor.Releaser())
					assert.Equal(t, originalAnnotations, accessor.Annotations())
					accessor.SetStatus(common.StatusDeployed)
					require.NoError(t, storage.Update(ctx, accessor))
					accessor, err = storage.GetRelease(ctx, "app", 1)
					require.NoError(t, err)
					assert.Equal(t, string(common.StatusDeployed), accessor.Status())
					assert.Equal(t, originalAnnotations, accessor.Annotations())
					require.NoError(t, storage.UpdateLabels(ctx, "app", 1, map[string]string{"team": "label"}))
					stored, err = backend.get(ctx, "ns", storageKey("app", 1))
					require.NoError(t, err)
					assert.Equal(t, originalAnnotations, stored.Annotations)

					updated := releaseCase.newRelease(map[string]string{"team": "new"})
					updatedAccessor, err := helmrel.NewAccessor(updated)
					require.NoError(t, err)
					updatedAccessor.SetStatus(common.StatusDeployed)
					require.NoError(t, storage.Update(ctx, updatedAccessor))
					assert.Equal(t, map[string]string{"team": "new"}, updatedAccessor.Annotations())

					accessor, err = storage.GetRelease(ctx, "app", 1)
					require.NoError(t, err)
					assert.Equal(t, map[string]string{
						"team":                         "new",
						storageAnnotationReleaseFormat: storageAnnotationReleaseFormatV1,
					}, accessor.Annotations())

					stored, err = backend.get(ctx, "ns", storageKey("app", 1))
					require.NoError(t, err)
					body, err = decodeRelease(stored.Body)
					require.NoError(t, err)
					assert.Equal(t, stored.Annotations, body.Info.Annotations)

					updated = releaseCase.newRelease(nil)
					updatedAccessor, err = helmrel.NewAccessor(updated)
					require.NoError(t, err)
					updatedAccessor.SetStatus(common.StatusDeployed)
					require.NoError(t, storage.Update(ctx, updatedAccessor))
					assert.Nil(t, updatedAccessor.Annotations())

					accessor, err = storage.GetRelease(ctx, "app", 1)
					require.NoError(t, err)
					assert.Equal(t, map[string]string{storageAnnotationReleaseFormat: storageAnnotationReleaseFormatV1}, accessor.Annotations())

					stored, err = backend.get(ctx, "ns", storageKey("app", 1))
					require.NoError(t, err)
					body, err = decodeRelease(stored.Body)
					require.NoError(t, err)
					assert.Equal(t, accessor.Annotations(), body.Info.Annotations)
					assert.Equal(t, stored.Annotations, body.Info.Annotations)
				})
			}
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

func TestReleaseStorage_KubeOlderWriterPreservesBodyAnnotations(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			s := newTestKubeStorage(t, kind, "ns", 0)
			rls := newTestRelease("ns", "app", 1, common.StatusPendingInstall)
			rls.Info.Annotations = map[string]string{"team": "platform"}
			accessor, err := helmrel.NewAccessor(rls)
			require.NoError(t, err)
			require.NoError(t, s.storage.Create(ctx, accessor))

			expected := map[string]string{storageAnnotationReleaseFormat: storageAnnotationReleaseFormatV1, "team": "platform"}

			var older driver.Driver
			switch kind {
			case kubeStorageKindSecret:
				older = driver.NewSecrets(s.client.CoreV1().Secrets("ns"))
			case kubeStorageKindConfigMap:
				older = driver.NewConfigMaps(s.client.CoreV1().ConfigMaps("ns"))
			}

			olderRelease, err := older.Get(storageKey("app", 1))
			require.NoError(t, err)
			olderAccessor, err := helmrel.NewAccessor(olderRelease)
			require.NoError(t, err)
			require.Equal(t, expected, olderAccessor.Annotations())
			olderAccessor.SetStatus(common.StatusDeployed)
			require.NoError(t, older.Update(storageKey("app", 1), olderRelease))

			stored, err := s.backend.get(ctx, "ns", storageKey("app", 1))
			require.NoError(t, err)
			assert.Empty(t, stored.Annotations)

			loaded, err := s.storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			assert.Equal(t, string(common.StatusDeployed), loaded.Status())
			assert.Equal(t, expected, loaded.Annotations())

			olderAccessor.Annotations()["team"] = "older"
			olderAccessor.Annotations()["extra"] = "value"

			require.NoError(t, older.Update(storageKey("app", 1), olderRelease))

			expected["team"] = "older"
			expected["extra"] = "value"
			loaded, err = s.storage.GetRelease(ctx, "app", 1)
			require.NoError(t, err)
			assert.Equal(t, expected, loaded.Annotations())

			summary, err := s.storage.LoadRevisionSummary(ctx, Revision{Name: "app", Namespace: "ns", Version: 1})
			require.NoError(t, err)
			assert.Equal(t, expected, summary.Annotations)

			loaded.SetStatus(common.StatusSuperseded)
			require.NoError(t, s.storage.Update(ctx, loaded))
			stored, err = s.backend.get(ctx, "ns", storageKey("app", 1))
			require.NoError(t, err)
			assert.Equal(t, expected, stored.Annotations)
			body, err := decodeRelease(stored.Body)
			require.NoError(t, err)
			assert.Equal(t, expected, body.Info.Annotations)
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
				"reserved":    {storageAnnotationReleaseFormat: "user"},
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

func TestStoredObject_AnnotationsDuplicatedWithoutMutatingRelease(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{"team": "platform"}

	obj, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.NoError(t, err)

	body, err := decodeRelease(obj.Body)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"nelm.werf.io/release-format": "v1", "team": "platform"}, body.Info.Annotations)
	assert.Equal(t, obj.Annotations, body.Info.Annotations)
	assert.Equal(t, map[string]string{"team": "platform"}, rls.Info.Annotations)
}

func TestStoredObject_EmptyAnnotationsHaveMarker(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	obj, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"nelm.werf.io/release-format": "v1"}, obj.Annotations)
	assert.Nil(t, rls.Info.Annotations)

	body, err := decodeRelease(obj.Body)
	require.NoError(t, err)
	assert.Equal(t, obj.Annotations, body.Info.Annotations)

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

	legacy.Annotations = map[string]string{storageAnnotationReleaseFormat: storageAnnotationReleaseFormatV1, "team": "metadata"}
	loaded, err = decodeStoredObject(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy.Annotations, loaded.Info.Annotations)
	summary, err = decodeStoredSummary(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy.Annotations, summary.Annotations)
	loaded.Info.Annotations["team"] = "changed"
	assert.Equal(t, "metadata", legacy.Annotations["team"])
}

func TestStoredObject_ReleaseFormatControlsMetadataAnnotations(t *testing.T) {
	for _, tt := range []struct {
		name        string
		annotations map[string]string
		known       bool
	}{
		{name: "v1", annotations: map[string]string{"nelm.werf.io/release-format": "v1", "team": "metadata"}, known: true},
		{name: "absent", annotations: map[string]string{"team": "metadata"}},
		{name: "unknown", annotations: map[string]string{"nelm.werf.io/release-format": "v2", "team": "metadata"}},
		{name: "empty", annotations: map[string]string{"nelm.werf.io/release-format": "", "team": "metadata"}},
		{name: "old writer marker", annotations: map[string]string{"nelm.werf.io/released-by": "nelm", "team": "metadata"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
			rls.Info.Annotations = map[string]string{"team": "body"}
			obj := newTestStoredObject(t, rls)
			obj.Body = encodeHelmRelease(t, rls)
			obj.Annotations = tt.annotations

			loaded, err := decodeStoredObject(obj)
			require.NoError(t, err)
			summary, err := decodeStoredSummary(obj)
			require.NoError(t, err)
			revision, ok, err := revisionFromStoredObject(obj)
			require.NoError(t, err)
			require.True(t, ok)

			if tt.known {
				assert.Equal(t, tt.annotations, loaded.Info.Annotations)
				assert.Equal(t, tt.annotations, summary.Annotations)
				assert.Equal(t, tt.annotations, revision.Annotations)
			} else {
				assert.Equal(t, rls.Info.Annotations, loaded.Info.Annotations)
				assert.Equal(t, rls.Info.Annotations, summary.Annotations)
				assert.Nil(t, revision.Annotations)
			}

			rls.Info.Annotations = tt.annotations
			accessor, err := helmrel.NewAccessor(rls)
			require.NoError(t, err)

			revision = NewRevisionFromAccessor(accessor)
			if tt.known {
				assert.Equal(t, tt.annotations, revision.Annotations)
			} else {
				assert.Nil(t, revision.Annotations)
			}
		})
	}
}

func TestStoredObject_ReservedReleaseFormatCannotBeChanged(t *testing.T) {
	rls := newTestRelease("ns", "app", 1, common.StatusDeployed)
	rls.Info.Annotations = map[string]string{storageAnnotationReleaseFormat: "user"}
	_, err := newStoredObject("ns", rls, storageLabelCreatedAt)
	require.ErrorContains(t, err, "reserved")
}
