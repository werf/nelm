package resourceparts

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ResourceScope string

const (
	ResourceScopeNamespace ResourceScope = "namespace"
	ResourceScopeCluster   ResourceScope = "cluster"
	ResourceScopeUnknown   ResourceScope = "unknown"
)

func NewLocalBaseResource(unstruct *unstructured.Unstructured, filePath string, opts NewLocalBaseResourceOptions) *LocalBaseResource {
	return &LocalBaseResource{
		unstructured: unstruct,
		filePath:     filePath,
		mapper:       opts.Mapper,
	}
}

type NewLocalBaseResourceOptions struct {
	Mapper meta.ResettableRESTMapper
}

type LocalBaseResource struct {
	unstructured *unstructured.Unstructured
	filePath     string
	mapper       meta.ResettableRESTMapper
}

func (r *LocalBaseResource) Validate() error {
	return nil
}

func (r *LocalBaseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *LocalBaseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *LocalBaseResource) FilePath() string {
	return r.filePath
}

func (r *LocalBaseResource) Scope() (ResourceScope, error) {
	if r.mapper != nil {
		mapping, err := r.mapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if err != nil {
			return ResourceScopeUnknown, fmt.Errorf("error getting resource mapping for %q: %w", r.GroupVersionKind(), err)
		}

		if mapping.Scope == meta.RESTScopeNamespace {
			return ResourceScopeNamespace, nil
		} else {
			return ResourceScopeCluster, nil
		}
	}

	return ResourceScopeUnknown, nil
}

func (r *LocalBaseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *LocalBaseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}

func (r *LocalBaseResource) String() string {
	if r.Namespace() != "" {
		return fmt.Sprintf("%s:%s/%s", r.GroupVersionKind().Kind, r.Namespace(), r.Name())
	} else {
		return fmt.Sprintf("%s:%s", r.GroupVersionKind().Kind, r.Name())
	}
}
