package resourcev2

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyHumanResourcePolicy = "helm.sh/resource-policy"
var annotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

func newNeverDeletableResource(unstruct *unstructured.Unstructured) *neverDeletableResource {
	return &neverDeletableResource{unstructured: unstruct}
}

type neverDeletableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *neverDeletableResource) Validate() error {
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

func (r *neverDeletableResource) ShouldNeverBeDeleted() bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternResourcePolicy)
	if !found {
		return false
	}

	return value == "keep"
}
