package resource

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// FIXME(ilya-lesikov): do we need this? Too complex
var _ ResourcedReferencer = (*ResourcedReference)(nil)

type ResourcedReferenceOptions struct {
	GroupVersionKind     schema.GroupVersionKind
	GroupVersionResource schema.GroupVersionResource
}

func NewResourcedReference(name, namespace string, options ResourcedReferenceOptions) *ResourcedReference {
	if options.GroupVersionKind == (schema.GroupVersionKind{}) && options.GroupVersionResource == (schema.GroupVersionResource{}) {
		panic("groupVersionKind and groupVersionResource can't be both empty")
	}

	return &ResourcedReference{
		Reference:            NewReference(name, namespace, options.GroupVersionKind),
		groupVersionResource: options.GroupVersionResource,
	}
}

// One of groupVersionKind or groupVersionResource might be empty.
type ResourcedReference struct {
	*Reference
	groupVersionResource schema.GroupVersionResource
}

func (r *ResourcedReference) Validate() error {
	return nil
}

func (r *ResourcedReference) Name() string {
	return r.name
}

func (r *ResourcedReference) Namespace() string {
	return r.namespace
}

func (r *ResourcedReference) GroupVersionResource() schema.GroupVersionResource {
	return r.groupVersionResource
}

func (r *ResourcedReference) Matches(other Referencer) bool {
	return r.Name() == other.Name() && r.Namespace() == other.Namespace() && r.GroupVersionKind() == other.GroupVersionKind()
}

func (r *ResourcedReference) MatchesResourced(other ResourcedReferencer) bool {
	return r.Name() == other.Name() && r.Namespace() == other.Namespace() && r.GroupVersionResource() == other.GroupVersionResource()
}

func (r *ResourcedReference) HasGroupVersionKind() bool {
	return r.GroupVersionKind() != (schema.GroupVersionKind{})
}

func (r *ResourcedReference) HasGroupVersionResource() bool {
	return r.GroupVersionResource() != (schema.GroupVersionResource{})
}

func (r *ResourcedReference) String() string {
	var gvxString string
	if r.HasGroupVersionKind() {
		gvxString = r.GroupVersionKind().String()
	} else {
		gvxString = r.GroupVersionResource().String()
	}

	var resultParts []string
	for _, part := range []string{gvxString, r.Namespace(), r.Name()} {
		if part == "" {
			continue
		}

		resultParts = append(resultParts, part)
	}

	return strings.Join(resultParts, "/")
}
