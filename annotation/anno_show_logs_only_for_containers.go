package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)

var _ Annotationer = (*AnnotationShowLogsOnlyForContainers)(nil)

func NewAnnotationShowLogsOnlyForContainers(key, value string) *AnnotationShowLogsOnlyForContainers {
	return &AnnotationShowLogsOnlyForContainers{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationShowLogsOnlyForContainers struct{ *baseAnnotation }

func (a *AnnotationShowLogsOnlyForContainers) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if strings.Contains(strings.TrimSpace(a.Value()), ",") {
		for _, container := range strings.Split(strings.TrimSpace(a.Value()), ",") {
			if strings.TrimSpace(container) == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated containers has no name", a.Value(), a.Key())
			}

			// TODO(ilya-lesikov): should be valid container name
		}
	}

	return nil
}

func (a *AnnotationShowLogsOnlyForContainers) ForContainers() []string {
	return strings.Split(strings.TrimSpace(a.Value()), ",")
}
