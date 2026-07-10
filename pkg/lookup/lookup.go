package lookup

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"

	"github.com/werf/nelm/pkg/helm/pkg/engine"
)

var _ engine.ClientProvider = (*LocalClientProvider)(nil)

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

	return p.client.Resource(gvr), p.namespaced[gvk], nil
}
