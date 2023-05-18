package resource

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Resourcer interface {
	Referencer
	Unstructured() *unstructured.Unstructured
	PartOfRelease() bool
	DeepCopy() Resourcer
}

type Referencer interface {
	Validate() error
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	Matches(other Referencer) bool
	String() string
}

type ResourcedReferencer interface {
	Referencer
	GroupVersionResource() schema.GroupVersionResource
	MatchesResourced(other ResourcedReferencer) bool
	HasGroupVersionKind() bool
	HasGroupVersionResource() bool
}

func CastToResourcers[T Resourcer](resourcers []T) []Resourcer {
	result := []Resourcer{}
	for _, h := range resourcers {
		result = append(result, h)
	}
	return result
}

func CastToReferencers[T Referencer](referencers []T) []Referencer {
	result := []Referencer{}
	for _, h := range referencers {
		result = append(result, h)
	}
	return result
}

func CastToResourcedReferencers[T ResourcedReferencer](referencers []T) []ResourcedReferencer {
	result := []ResourcedReferencer{}
	for _, h := range referencers {
		result = append(result, h)
	}
	return result
}
