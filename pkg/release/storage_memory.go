package release

import (
	"context"
	"maps"
	"slices"
	"sort"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
)

var _ storageBackend = (*memoryStorageBackend)(nil)

// memoryStorageBackend keeps encoded revisions in process memory, keyed by namespace and
// object name, so it exercises the same encoding as the persistent backends.
type memoryStorageBackend struct {
	objects *kdutil.Concurrent[map[string]*storedObject]
}

func newMemoryStorageBackend() *memoryStorageBackend {
	return &memoryStorageBackend{
		objects: kdutil.NewConcurrent(map[string]*storedObject{}),
	}
}

func (b *memoryStorageBackend) create(_ context.Context, obj *storedObject) error {
	var err error

	b.objects.RWTransaction(func(objects map[string]*storedObject) {
		id := memoryStorageID(obj.Namespace, obj.Key)
		if _, found := objects[id]; found {
			err = ErrReleaseExists

			return
		}

		objects[id] = cloneStoredObject(obj, true)
	})

	return err
}

func (b *memoryStorageBackend) delete(_ context.Context, namespace, key string) error {
	var err error

	b.objects.RWTransaction(func(objects map[string]*storedObject) {
		id := memoryStorageID(namespace, key)
		if _, found := objects[id]; !found {
			err = ErrReleaseNotFound

			return
		}

		delete(objects, id)
	})

	return err
}

func (b *memoryStorageBackend) get(_ context.Context, namespace, key string) (*storedObject, error) {
	var result *storedObject

	b.objects.RTransaction(func(objects map[string]*storedObject) {
		if obj, found := objects[memoryStorageID(namespace, key)]; found {
			result = cloneStoredObject(obj, true)
		}
	})

	if result == nil {
		return nil, ErrReleaseNotFound
	}

	return result, nil
}

func (b *memoryStorageBackend) list(namespace, releaseName string, withBodies bool) []*storedObject {
	var result []*storedObject

	b.objects.RTransaction(func(objects map[string]*storedObject) {
		for _, obj := range objects {
			if namespace != "" && obj.Namespace != namespace {
				continue
			}

			if obj.Labels[storageLabelOwner] != storageOwner {
				continue
			}

			if releaseName != "" && obj.Labels[storageLabelName] != releaseName {
				continue
			}

			result = append(result, cloneStoredObject(obj, withBodies))
		}
	})

	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}

		return result[i].Key < result[j].Key
	})

	return result
}

func (b *memoryStorageBackend) listMetadata(_ context.Context, namespace, releaseName string) ([]*storedObject, error) {
	return b.list(namespace, releaseName, false), nil
}

func (b *memoryStorageBackend) listWithBodies(_ context.Context, namespace, releaseName string, fn func(obj *storedObject) error) error {
	for _, obj := range b.list(namespace, releaseName, true) {
		if err := fn(obj); err != nil {
			return err
		}
	}

	return nil
}

func (b *memoryStorageBackend) update(_ context.Context, obj *storedObject) error {
	var err error

	b.objects.RWTransaction(func(objects map[string]*storedObject) {
		id := memoryStorageID(obj.Namespace, obj.Key)
		if _, found := objects[id]; !found {
			err = ErrReleaseNotFound

			return
		}

		objects[id] = cloneStoredObject(obj, true)
	})

	return err
}

func (b *memoryStorageBackend) updateLabels(_ context.Context, namespace, key string, labels map[string]string) error {
	var err error

	b.objects.RWTransaction(func(objects map[string]*storedObject) {
		obj, found := objects[memoryStorageID(namespace, key)]
		if !found {
			err = ErrReleaseNotFound

			return
		}

		maps.Copy(obj.Labels, labels)
	})

	return err
}

func cloneStoredObject(obj *storedObject, withBody bool) *storedObject {
	clone := &storedObject{
		Namespace: obj.Namespace,
		Key:       obj.Key,
		Labels:    maps.Clone(obj.Labels),
	}

	if clone.Labels == nil {
		clone.Labels = map[string]string{}
	}

	if withBody {
		clone.Body = slices.Clone(obj.Body)
	}

	return clone
}

func memoryStorageID(namespace, key string) string {
	return namespace + "/" + key
}
