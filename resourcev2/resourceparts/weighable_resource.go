package resourceparts

import (
	"fmt"
	"regexp"
	"strconv"

	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)
var annotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

func NewWeighableResource(unstruct *unstructured.Unstructured) *WeighableResource {
	return &WeighableResource{unstructured: unstruct}
}

type WeighableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *WeighableResource) Validate() error {
	if IsHook(r.unstructured.GetAnnotations()) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHookWeight); found {
			if value == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty integer value", value, key)
			}

			if _, err := strconv.Atoi(value); err != nil {
				return errors.NewValidationError("invalid value %q for annotation %q, expected integer value", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternWeight); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		}
	}

	return nil
}

func (r *WeighableResource) Weight() int {
	var weightValue string
	if IsHook(r.unstructured.GetAnnotations()) {
		_, hookWeightValue, hookWeightFound := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHookWeight)

		_, generalWeightValue, weightFound := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternWeight)

		if !hookWeightFound && !weightFound {
			return 0
		} else if weightFound {
			weightValue = generalWeightValue
		} else {
			weightValue = hookWeightValue
		}
	} else {
		var found bool
		_, weightValue, found = FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternWeight)
		if !found {
			return 0
		}
	}

	weight, _ := strconv.Atoi(weightValue)

	return weight
}
