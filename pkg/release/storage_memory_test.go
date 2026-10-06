package release

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func TestMemoryStorageBackend_ListLatestWithBodiesConcurrent(t *testing.T) {
	ctx := context.Background()

	backend := newMemoryStorageBackend()
	for i := 0; i < 10; i++ {
		require.NoError(t, backend.create(ctx, &storedObject{Namespace: "ns", Key: fmt.Sprint(i), Labels: map[string]string{"owner": "helm", "packageChecksum": "yes"}, Body: []byte("body")}))
	}

	selector, err := labels.Parse("packageChecksum")
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			assert.NoError(t, backend.updateLabels(ctx, "ns", fmt.Sprint(i%10), map[string]string{"packageChecksum": fmt.Sprint(i)}))
			assert.NoError(t, backend.update(ctx, &storedObject{Namespace: "ns", Key: fmt.Sprint(i % 10), Labels: map[string]string{"owner": "helm", "packageChecksum": "yes"}, Body: []byte("body")}))
		}
	}()
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			assert.NoError(t, backend.listLatestWithBodies(ctx, "", selector, func(obj *storedObject) error {
				assert.Equal(t, "body", string(obj.Body))
				obj.Body[0] = 'x'
				obj.Labels["packageChecksum"] = "callback"

				return backend.updateLabels(ctx, obj.Namespace, obj.Key, map[string]string{"callback": "yes"})
			}))
		}
	}()

	wg.Wait()
}

func TestMemoryStorageBackend_ListLatestWithBodiesSelectors(t *testing.T) {
	backend := newMemoryStorageBackend()
	testLatestBodySelectors(t, func(obj *storedObject) {
		require.NoError(t, backend.create(context.Background(), obj))
	}, backend.listLatestWithBodies)
}

func TestMemoryStorageBackend_ListLatestWithBodiesSnapshot(t *testing.T) {
	ctx := context.Background()
	backend := newMemoryStorageBackend()
	input := &storedObject{Namespace: "ns", Key: "b", Labels: map[string]string{"owner": "helm", "packageChecksum": "old"}, Body: []byte("old")}
	require.NoError(t, backend.create(ctx, input))
	input.Body[0] = 'x'
	input.Labels["packageChecksum"] = "input-mutated"
	require.NoError(t, backend.create(ctx, &storedObject{Namespace: "ns", Key: "a", Labels: map[string]string{"owner": "helm"}, Body: []byte("a")}))
	require.NoError(t, backend.create(ctx, &storedObject{Namespace: "other", Key: "c", Labels: map[string]string{"owner": "helm"}, Body: []byte("c")}))

	var keys []string
	require.NoError(t, backend.listLatestWithBodies(ctx, "", labels.Everything(), func(obj *storedObject) error {
		keys = append(keys, obj.Namespace+"/"+obj.Key)
		switch obj.Key {
		case "a":
			require.NoError(t, backend.updateLabels(ctx, "ns", "b", map[string]string{"packageChecksum": "new"}))
			require.NoError(t, backend.update(ctx, &storedObject{Namespace: "ns", Key: "b", Labels: map[string]string{"owner": "helm"}, Body: []byte("new")}))
		case "b":
			assert.Equal(t, "old", string(obj.Body))
			assert.Equal(t, "old", obj.Labels["packageChecksum"])
		}

		obj.Labels["owner"] = "callback-mutated"
		obj.Body[0] = 'x'

		return nil
	}))
	assert.Equal(t, []string{"ns/a", "ns/b", "other/c"}, keys)

	stored, err := backend.get(ctx, "ns", "a")
	require.NoError(t, err)
	assert.Equal(t, "helm", stored.Labels["owner"])
	assert.Equal(t, "a", string(stored.Body))
	stored, err = backend.get(ctx, "ns", "b")
	require.NoError(t, err)
	assert.Equal(t, "new", string(stored.Body))
}

func TestMemoryStorageBackend_ListLatestWithBodiesStops(t *testing.T) {
	backend := newMemoryStorageBackend()

	ctx := context.Background()
	for _, key := range []string{"a", "b"} {
		require.NoError(t, backend.create(ctx, &storedObject{Namespace: "ns", Key: key, Labels: map[string]string{"owner": "helm"}}))
	}

	stop := errors.New("stop")
	calls := 0
	err := backend.listLatestWithBodies(ctx, "", labels.Everything(), func(*storedObject) error {
		calls++

		return stop
	})
	require.Same(t, stop, err)
	assert.Equal(t, 1, calls)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = backend.listLatestWithBodies(ctx, "", labels.Everything(), func(*storedObject) error {
		t.Fatal("callback after cancellation")

		return nil
	})
	require.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	calls = 0
	err = backend.listLatestWithBodies(ctx, "", labels.Everything(), func(*storedObject) error {
		calls++

		cancel()

		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}
