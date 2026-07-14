package lookup

import (
	"context"
	"fmt"
	"strings"

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
}

// NewLocalClientProvider returns an engine.ClientProvider that resolves lookup calls against the
// provided in-memory set of Kubernetes objects instead of a live cluster. An empty set makes
// every lookup return an empty result, matching the offline stub behavior.
func NewLocalClientProvider(objects []*unstructured.Unstructured) *LocalClientProvider {
	runtimeObjects := make([]runtime.Object, 0, len(objects))

	namespaced := make(map[schema.GroupVersionKind]bool)
	for _, obj := range objects {
		runtimeObjects = append(runtimeObjects, obj)
		if obj.GetNamespace() != "" {
			namespaced[obj.GroupVersionKind()] = true
		}
	}

	return &LocalClientProvider{
		client:     fake.NewSimpleDynamicClient(runtime.NewScheme(), runtimeObjects...),
		namespaced: namespaced,
	}
}

func (p *LocalClientProvider) GetClientFor(apiVersion, kind string) (dynamic.NamespaceableResourceInterface, bool, error) {
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)

	return &emptyListResource{
		NamespaceableResourceInterface: p.client.Resource(gvr),
		listGVK:                        gvk.GroupVersion().WithKind(gvk.Kind + "List"),
	}, p.namespaced[gvk], nil
}

// emptyListResource wraps a fake dynamic resource interface so that a LIST for a kind not registered
// with the fake client returns an empty list instead of panicking with the fake's
// "you must register resource to list kind" coding error, preserving the offline stub semantics
// where a lookup for an absent kind yields an empty result.
type emptyListResource struct {
	dynamic.NamespaceableResourceInterface

	listGVK schema.GroupVersionKind
}

func (r *emptyListResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return listOrEmpty(ctx, r.NamespaceableResourceInterface, r.listGVK, opts)
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

func (r *emptyListNamespacedResource) List(ctx context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return listOrEmpty(ctx, r.ResourceInterface, r.listGVK, opts)
}

func listOrEmpty(ctx context.Context, delegate dynamic.ResourceInterface, listGVK schema.GroupVersionKind, opts metav1.ListOptions) (list *unstructured.UnstructuredList, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			msg, ok := rec.(string)
			if !ok || !strings.Contains(msg, "you must register resource to list kind") {
				panic(rec)
			}

			list = &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(listGVK)

			err = nil
		}
	}()

	list, err = delegate.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}

	return list, nil
}
