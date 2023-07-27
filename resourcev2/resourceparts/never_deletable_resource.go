package resourceparts

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

func NewNeverDeletableResource(unstruct *unstructured.Unstructured) *NeverDeletableResource {
	return &NeverDeletableResource{unstructured: unstruct}
}

type NeverDeletableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *NeverDeletableResource) Validate() error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternResourcePolicy); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case "keep":
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func (r *NeverDeletableResource) ShouldNeverBeDeleted() bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternResourcePolicy)
	if !found {
		return false
	}

	return value == "keep"
}
