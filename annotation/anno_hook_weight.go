package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

var _ Annotationer = (*AnnotationHookWeight)(nil)

func NewAnnotationHookWeight(key, value string) *AnnotationHookWeight {
	return &AnnotationHookWeight{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationHookWeight struct{ *baseAnnotation }

func (a *AnnotationHookWeight) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", a.Value(), a.Key())
	}

	_, err := strconv.Atoi(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected integer value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationHookWeight) HookWeight() int {
	result, _ := strconv.Atoi(strings.TrimSpace(a.Value()))
	return result
}
