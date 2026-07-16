package chart

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	"github.com/werf/nelm/pkg/helm/pkg/engine"
)

var (
	_ engine.ClientProvider                  = (*LocalClientProvider)(nil)
	_ dynamic.NamespaceableResourceInterface = (*emptyListResource)(nil)
)

type LocalClientProvider struct {
	client     *fake.FakeDynamicClient
	namespaced map[schema.GroupVersionKind]bool
	registered map[schema.GroupVersionKind]bool
}

// newLocalClientProvider returns an engine.ClientProvider that resolves lookup calls against the
// provided in-memory set of Kubernetes objects instead of a live cluster. An empty set makes
// every lookup return an empty result, matching the offline stub behavior.
func newLocalClientProvider(objects []*unstructured.Unstructured) *LocalClientProvider {
	runtimeObjects := make([]runtime.Object, 0, len(objects))

	registered := make(map[schema.GroupVersionKind]bool)

	namespaced := make(map[schema.GroupVersionKind]bool)
	for _, obj := range objects {
		runtimeObjects = append(runtimeObjects, obj)
		if obj.GetNamespace() != "" {
			namespaced[obj.GroupVersionKind()] = true
		}

		registered[obj.GroupVersionKind()] = true
	}

	return &LocalClientProvider{
		client:     fake.NewSimpleDynamicClient(runtime.NewScheme(), runtimeObjects...),
		namespaced: namespaced,
		registered: registered,
	}
}

func (p *LocalClientProvider) GetClientFor(apiVersion, kind string) (dynamic.NamespaceableResourceInterface, bool, error) {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)

	if !p.registered[gvk] {
		return &emptyListResource{
			NamespaceableResourceInterface: p.client.Resource(gvr),
			listGVK:                        gvk.GroupVersion().WithKind(gvk.Kind + "List"),
		}, p.namespaced[gvk], nil
	}

	return p.client.Resource(gvr), p.namespaced[gvk], nil
}

// emptyListResource wraps a fake dynamic resource interface for a kind with no registered objects,
// so that List returns an empty list (the fake client would otherwise panic for an unregistered
// list kind), matching the offline stub semantics where an absent kind yields an empty result.
type emptyListResource struct {
	dynamic.NamespaceableResourceInterface

	listGVK schema.GroupVersionKind
}

func (r *emptyListResource) List(_ context.Context, _ metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return listEmpty(r.listGVK)
}

func (r *emptyListResource) Namespace(namespace string) dynamic.ResourceInterface {
	return &emptyListNamespacedResource{
		ResourceInterface: r.NamespaceableResourceInterface.Namespace(namespace),
		listGVK:           r.listGVK,
	}
}

type emptyListNamespacedResource struct {
	dynamic.ResourceInterface

	listGVK schema.GroupVersionKind
}

func (r *emptyListNamespacedResource) List(_ context.Context, _ metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return listEmpty(r.listGVK)
}

func listEmpty(listGVK schema.GroupVersionKind) (*unstructured.UnstructuredList, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(listGVK)

	return list, nil
}
