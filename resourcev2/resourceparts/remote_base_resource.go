package resourceparts

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewRemoteBaseResource(unstruct *unstructured.Unstructured) *RemoteBaseResource {
	return &RemoteBaseResource{
		unstructured: unstruct,
	}
}

type RemoteBaseResource struct {
	unstructured *unstructured.Unstructured
}

func (r *RemoteBaseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *RemoteBaseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *RemoteBaseResource) Scope() (ResourceScope, error) {
	if r.unstructured.GetNamespace() != "" {
		return ResourceScopeNamespace, nil
	} else {
		return ResourceScopeCluster, nil
	}
}

func (r *RemoteBaseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *RemoteBaseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}

func (r *RemoteBaseResource) String() string {
	if r.Namespace() != "" {
		return fmt.Sprintf("%s:%s/%s", r.GroupVersionKind().Kind, r.Namespace(), r.Name())
	} else {
		return fmt.Sprintf("%s:%s", r.GroupVersionKind().Kind, r.Name())
	}
}
