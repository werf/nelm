package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)

var _ Annotationer = (*AnnotationWeight)(nil)

func NewAnnotationWeight(key, value string) *AnnotationWeight {
	return &AnnotationWeight{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationWeight struct{ *baseAnnotation }

func (a *AnnotationWeight) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", a.Value(), a.Key())
	}

	_, err := strconv.Atoi(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected integer value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationWeight) Weight() int {
	result, _ := strconv.Atoi(strings.TrimSpace(a.Value()))
	return result
}
