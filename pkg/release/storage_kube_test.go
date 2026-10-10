package release

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubeStorageBackend_ListBodiesPagination(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		for _, latest := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/latest=%t", kind, latest), func(t *testing.T) {
				s := newTestKubeStorage(t, kind, "", 0)

				var (
					retained *storedObject
					calls    int
				)

				s.client.PrependReactor("list", s.backend.gvr().Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
					opts := action.(k8stesting.ListActionImpl).GetListOptions()
					assert.EqualValues(t, kubeStoragePageSize, opts.Limit)
					assert.Empty(t, action.GetNamespace())

					selector, err := labels.Parse(opts.LabelSelector)
					require.NoError(t, err)
					assert.False(t, selector.Matches(labels.Set{"owner": "other", "name": "a", "version": "2", "packageChecksum": "yes"}))
					assert.True(t, selector.Matches(labels.Set{"owner": "helm", "name": "a", "version": "2", "packageChecksum": "yes"}))

					if latest {
						assert.False(t, selector.Matches(labels.Set{"owner": "helm"}))
					} else {
						assert.False(t, selector.Matches(labels.Set{"owner": "helm", "name": "a", "version": "1"}))
						assert.False(t, selector.Matches(labels.Set{"owner": "helm", "name": "b", "version": "2"}))
					}

					calls++

					meta := metav1.ListMeta{Continue: "next"}
					if calls == 1 {
						assert.Empty(t, opts.Continue)
					} else {
						assert.Equal(t, "next", opts.Continue)
						meta.Continue = ""

						require.NotNil(t, retained)
						assert.Equal(t, "body-1", string(retained.Body))
					}

					obj := &storedObject{Namespace: "ns", Key: fmt.Sprintf("key-%d", calls), Labels: map[string]string{"owner": "helm", "name": "a", "version": "2", "packageChecksum": "yes"}, Body: []byte(fmt.Sprintf("body-%d", calls))}
					if kind == kubeStorageKindSecret {
						return true, &corev1.SecretList{ListMeta: meta, Items: []corev1.Secret{*newKubeSecret(obj)}}, nil
					}

					return true, &corev1.ConfigMapList{ListMeta: meta, Items: []corev1.ConfigMap{*newKubeConfigMap(obj)}}, nil
				})

				var keys []string

				fn := func(obj *storedObject) error {
					keys = append(keys, obj.Key)
					if retained == nil {
						retained = obj
					}

					return nil
				}

				var err error
				if latest {
					selector, parseErr := labels.Parse("packageChecksum")
					require.NoError(t, parseErr)

					err = s.backend.scanLatestCandidates(context.Background(), "", selector, true, fn)
				} else {
					err = s.backend.listWithBodies(context.Background(), "", "a", []int{2}, fn)
				}

				require.NoError(t, err)
				assert.Equal(t, []string{"key-1", "key-2"}, keys)
				assert.Equal(t, 2, calls)
				assert.Zero(t, countActions(s.client.Actions(), "get"))
				assert.Empty(t, s.metadataClient.Actions())
			})
		}
	}
}

func TestKubeStorageBackend_MetadataListingReportsCancellationDuringLastObject(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, "", 0)
	putTestKubeObject(t, s, &storedObject{Namespace: "ns", Key: "a-v1", Labels: map[string]string{"owner": "helm", "name": "a", "version": "1"}, Body: []byte("a-v1")})

	ctx, cancel := context.WithCancel(context.Background())
	err := s.backend.scanLatestCandidates(ctx, "", labels.Everything(), false, func(*storedObject) error {
		cancel()

		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestKubeStorageBackend_MetadataListingStopsOnCancellation(t *testing.T) {
	s := newTestKubeStorage(t, kubeStorageKindSecret, "", 0)
	putTestKubeObject(t, s, &storedObject{Namespace: "ns", Key: "a-v1", Labels: map[string]string{"owner": "helm", "name": "a", "version": "1"}, Body: []byte("a-v1")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.backend.scanLatestCandidates(ctx, "", labels.Everything(), false, func(*storedObject) error {
		t.Fatal("callback after cancellation")

		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, s.metadataClient.Actions())

	_, err = s.backend.listMetadata(ctx, "", "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestKubeStorageBackend_ScanLatestCandidatesWithBodiesSelectors(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			s := newTestKubeStorage(t, kind, "", 0)
			testLatestBodySelectors(t, func(obj *storedObject) {
				putTestKubeObject(t, s, obj)
			}, func(ctx context.Context, namespace string, selector labels.Selector, fn func(*storedObject) error) error {
				return s.backend.scanLatestCandidates(ctx, namespace, selector, true, fn)
			})
			assert.Zero(t, countActions(s.client.Actions(), "get"))
			assert.Empty(t, s.metadataClient.Actions())
		})
	}
}

func TestKubeStorageBackend_ScanLatestCandidatesWithBodiesStops(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			s := newTestKubeStorage(t, kind, "", 0)
			for _, key := range []string{"a", "b"} {
				putTestKubeObject(t, s, &storedObject{Namespace: "ns", Key: key, Labels: map[string]string{"owner": "helm"}, Body: []byte(key)})
			}

			stop := errors.New("stop")
			calls := 0
			err := s.backend.scanLatestCandidates(context.Background(), "", labels.Everything(), true, func(*storedObject) error {
				calls++

				return stop
			})
			require.Same(t, stop, err)
			assert.Equal(t, 1, calls)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err = s.backend.scanLatestCandidates(ctx, "", labels.Everything(), true, func(*storedObject) error {
				t.Fatal("callback after cancellation")

				return nil
			})
			require.ErrorIs(t, err, context.Canceled)

			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()

			calls = 0
			err = s.backend.scanLatestCandidates(ctx, "", labels.Everything(), true, func(*storedObject) error {
				calls++

				cancel()

				return nil
			})
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 1, calls)
			s.client.PrependReactor("list", s.backend.gvr().Resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, stop
			})
			err = s.backend.scanLatestCandidates(context.Background(), "", labels.Everything(), true, func(*storedObject) error { return nil })
			require.ErrorIs(t, err, stop)
		})
	}
}

func TestKubeStorageBackend_ScanLatestCandidatesWithoutBodies(t *testing.T) {
	for _, kind := range []kubeStorageKind{kubeStorageKindSecret, kubeStorageKindConfigMap} {
		t.Run(string(kind), func(t *testing.T) {
			s := newTestKubeStorage(t, kind, "", 0)
			putTestKubeObject(t, s, &storedObject{Namespace: "ns", Key: "a-v1", Labels: map[string]string{"owner": "helm", "name": "a", "version": "1", "packageChecksum": "x"}, Body: []byte("a-v1")})

			selector, err := labels.Parse("packageChecksum")
			require.NoError(t, err)

			var objects []*storedObject
			require.NoError(t, s.backend.scanLatestCandidates(context.Background(), "", selector, false, func(obj *storedObject) error {
				objects = append(objects, obj)

				return nil
			}))
			require.Len(t, objects, 1)
			assert.Nil(t, objects[0].Body)
			assert.Equal(t, "a-v1", objects[0].Key)

			listActions := lo.Filter(s.metadataClient.Actions(), func(action k8stesting.Action, _ int) bool { return action.GetVerb() == "list" })
			require.Len(t, listActions, 1)
			assert.Equal(t, "owner=helm,packageChecksum", listActions[0].(k8stesting.ListActionImpl).GetListRestrictions().Labels.String())
			assert.Zero(t, countActions(s.client.Actions(), "list"))
			assert.Zero(t, countActions(s.client.Actions(), "get"))
		})
	}
}
