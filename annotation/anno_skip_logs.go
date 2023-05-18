package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)

var _ Annotationer = (*AnnotationSkipLogs)(nil)

func NewAnnotationSkipLogs(key, value string) *AnnotationSkipLogs {
	return &AnnotationSkipLogs{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationSkipLogs struct{ *baseAnnotation }

func (a *AnnotationSkipLogs) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if _, err := strconv.ParseBool(strings.TrimSpace(a.Value())); err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationSkipLogs) SkipLogs() bool {
	result, _ := strconv.ParseBool(strings.TrimSpace(a.Value()))
	return result
}
