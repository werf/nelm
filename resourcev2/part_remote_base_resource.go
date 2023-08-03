package resourcev2

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newRemoteBaseResource(unstruct *unstructured.Unstructured) *remoteBaseResource {
	return &remoteBaseResource{
		unstructured: unstruct,
	}
}

type remoteBaseResource struct {
	unstructured *unstructured.Unstructured
}

func (r *remoteBaseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *remoteBaseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *remoteBaseResource) Scope() (ResourceScope, error) {
	if r.unstructured.GetNamespace() != "" {
		return ResourceScopeNamespace, nil
	} else {
		return ResourceScopeCluster, nil
	}
}

func (r *remoteBaseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *remoteBaseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}

func (r *remoteBaseResource) String() string {
	if r.Namespace() != "" {
		return fmt.Sprintf("%s:%s/%s", r.GroupVersionKind().Kind, r.Namespace(), r.Name())
	} else {
		return fmt.Sprintf("%s:%s", r.GroupVersionKind().Kind, r.Name())
	}
}
