package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)

var _ Annotationer = (*AnnotationLogRegex)(nil)

func NewAnnotationLogRegex(key, value string) *AnnotationLogRegex {
	return &AnnotationLogRegex{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationLogRegex struct{ *baseAnnotation }

func (a *AnnotationLogRegex) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if _, err := regexp.Compile(strings.TrimSpace(a.Value())); err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected valid regular expression", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationLogRegex) LogRegex() *regexp.Regexp {
	return regexp.MustCompile(strings.TrimSpace(a.Value()))
}
