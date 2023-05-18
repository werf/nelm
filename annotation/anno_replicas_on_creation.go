package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

var _ Annotationer = (*AnnotationReplicasOnCreation)(nil)

func NewAnnotationReplicasOnCreation(key, value string) *AnnotationReplicasOnCreation {
	return &AnnotationReplicasOnCreation{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationReplicasOnCreation struct{ *baseAnnotation }

func (a *AnnotationReplicasOnCreation) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	replicas, err := strconv.Atoi(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, value must be a number", a.Value(), a.Key())
	}

	if replicas < 0 {
		return fmt.Errorf("invalid value %q for annotation %q, value must be a positive number or zero", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationReplicasOnCreation) Replicas() int {
	result, _ := strconv.Atoi(strings.TrimSpace(a.Value()))
	return result
}
