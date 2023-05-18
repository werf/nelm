package annotation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var AnnotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)

var _ Annotationer = (*AnnotationShowServiceMessages)(nil)

func NewAnnotationShowServiceMessages(key, value string) *AnnotationShowServiceMessages {
	return &AnnotationShowServiceMessages{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationShowServiceMessages struct{ *baseAnnotation }

func (a *AnnotationShowServiceMessages) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	_, err := strconv.ParseBool(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationShowServiceMessages) ShowServiceMessages() bool {
	result, _ := strconv.ParseBool(strings.TrimSpace(a.Value()))
	return result
}
