package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)

var _ Annotationer = (*AnnotationFailuresAllowedPerReplica)(nil)

func NewAnnotationFailuresAllowedPerReplica(key, value string) *AnnotationFailuresAllowedPerReplica {
	return &AnnotationFailuresAllowedPerReplica{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationFailuresAllowedPerReplica struct{ *baseAnnotation }

func (a *AnnotationFailuresAllowedPerReplica) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", a.Value(), a.Key())
	}

	failuresAllowed, err := strconv.Atoi(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected integer value", a.Value(), a.Key())
	}

	if failuresAllowed < 0 {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationFailuresAllowedPerReplica) FailuresAllowedPerReplica() int {
	result, _ := strconv.Atoi(strings.TrimSpace(a.Value()))
	return result
}
