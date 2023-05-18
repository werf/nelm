package resource

import (
	"fmt"

	"helm.sh/helm/v3/pkg/werf/annotation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newBaseResource(u *unstructured.Unstructured) *baseResource {
	ref := NewReference(u.GetName(), u.GetNamespace(), u.GroupVersionKind())

	return &baseResource{
		Reference:    ref,
		unstructured: u,
	}
}

type baseResource struct {
	*Reference
	unstructured *unstructured.Unstructured
}

func (r *baseResource) Validate() error {
	for key, value := range r.unstructured.GetAnnotations() {
		if err := annotation.AnnotationFactory(key, value).Validate(); err != nil {
			return fmt.Errorf("error validating annotation: %w", err)
		}
	}

	// FIXME(ilya-lesikov): make sure there is no way to only specify external dependency namespace

	return nil
}

func (r *baseResource) Name() string {
	return r.unstructured.GetName()
}

func (r *baseResource) Namespace() string {
	return r.unstructured.GetNamespace()
}

func (r *baseResource) GroupVersionKind() schema.GroupVersionKind {
	return r.unstructured.GroupVersionKind()
}

func (r *baseResource) Unstructured() *unstructured.Unstructured {
	return r.unstructured
}
