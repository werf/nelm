package resource

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewGenericResource(unstructured *unstructured.Unstructured) *GenericResource {
	return &GenericResource{
		baseResource: newBaseResource(unstructured),
	}
}

type GenericResource struct {
	*baseResource
}

func (r *GenericResource) PartOfRelease() bool {
	return false
}

func (r *GenericResource) DeepCopy() Resourcer {
	return NewGenericResource(r.Unstructured().DeepCopy())
}
