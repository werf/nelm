package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)

var _ Annotationer = (*AnnotationSkipLogsForContainers)(nil)

func NewAnnotationSkipLogsForContainers(key, value string) *AnnotationSkipLogsForContainers {
	return &AnnotationSkipLogsForContainers{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationSkipLogsForContainers struct{ *baseAnnotation }

func (a *AnnotationSkipLogsForContainers) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if strings.Contains(strings.TrimSpace(a.Value()), ",") {
		for _, container := range strings.Split(strings.TrimSpace(a.Value()), ",") {
			if strings.TrimSpace(container) == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated containers has no name", a.Value(), a.Key())
			}

			// TODO(ilya-lesikov): validate container name
		}
	}

	return nil
}

func (a *AnnotationSkipLogsForContainers) ForContainers() []string {
	return strings.Split(strings.TrimSpace(a.Value()), ",")
}
