package resourcev2

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newLocalBaseResource(unstruct *unstructured.Unstructured, filePath string, opts newLocalBaseResourceOptions) *localBaseResource {
	return &localBaseResource{
		unstructured: unstruct,
		filePath:     filePath,
		mapper:       opts.Mapper,
	}
}

type newLocalBaseResourceOptions struct {
	Mapper meta.ResettableRESTMapper
}

type localBaseResource struct {
	unstructured *unstructured.Unstructured
	filePath     string
	mapper       meta.ResettableRESTMapper
}

func (r *localBaseResource) Validate() error {
	return nil
}

func (r *localBaseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *localBaseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *localBaseResource) FilePath() string {
	return r.filePath
}

func (r *localBaseResource) Scope() (ResourceScope, error) {
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

func (r *localBaseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *localBaseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}

func (r *localBaseResource) String() string {
	if r.Namespace() != "" {
		return fmt.Sprintf("%s:%s/%s", r.GroupVersionKind().Kind, r.Namespace(), r.Name())
	} else {
		return fmt.Sprintf("%s:%s", r.GroupVersionKind().Kind, r.Name())
	}
}
