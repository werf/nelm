package resourcev2

import (
	"fmt"
	"regexp"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyHumanReplicasOnCreation = "werf.io/replicas-on-creation"
var annotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

func newReplicableResource(unstruct *unstructured.Unstructured) *replicableResource {
	return &replicableResource{unstructured: unstruct}
}

type replicableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *replicableResource) Validate() error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReplicasOnCreation); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty numeric value", value, key)
		}

		replicas, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, value must be a number", value, key)
		}

		if replicas < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, value must be a positive number or zero", value, key)
		}
	}

	return nil
}

func (r *replicableResource) DefaultReplicasOnCreation() (replicas int, set bool) {
	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReplicasOnCreation)
	if !found {
		return 0, false
	}

	replicas, _ = strconv.Atoi(value)

	return replicas, true
}
