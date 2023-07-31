package resourcev2

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func IsCRD(unstruct *unstructured.Unstructured) bool {
	return unstruct.GroupVersionKind().GroupKind() == schema.GroupKind{
		Group: "apiextensions.k8s.io",
		Kind:  "CustomResourceDefinition",
	}
}
