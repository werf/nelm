package annotation

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var AnnotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)

var _ Annotationer = (*AnnotationNoActivityTimeout)(nil)

func NewAnnotationNoActivityTimeout(key, value string) *AnnotationNoActivityTimeout {
	return &AnnotationNoActivityTimeout{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationNoActivityTimeout struct{ *baseAnnotation }

func (a *AnnotationNoActivityTimeout) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty duration value", a.Value(), a.Key())
	}

	duration, err := time.ParseDuration(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected duration value", a.Value(), a.Key())
	}

	if duration.Seconds() < 0 {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-negative duration value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationNoActivityTimeout) NoActivityTimeout() time.Duration {
	duration, _ := time.ParseDuration(strings.TrimSpace(a.Value()))
	return duration
}
