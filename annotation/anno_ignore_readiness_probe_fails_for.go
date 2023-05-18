package annotation

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

var AnnotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)

var _ Annotationer = (*AnnotationIgnoreReadinessProbeFailsFor)(nil)

func NewAnnotationIgnoreReadinessProbeFailsFor(key, value string) *AnnotationIgnoreReadinessProbeFailsFor {
	return &AnnotationIgnoreReadinessProbeFailsFor{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationIgnoreReadinessProbeFailsFor struct{ *baseAnnotation }

func (a *AnnotationIgnoreReadinessProbeFailsFor) Validate() error {
	keyMatches := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(a.Key())
	if keyMatches == nil {
		return fmt.Errorf("unexpected annotation %q", a.Key())
	}

	containerSubexpIndex := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
	if containerSubexpIndex == -1 {
		return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternIgnoreReadinessProbeFailsFor.String(), a.Key())
	}

	if len(keyMatches) < containerSubexpIndex+1 {
		return fmt.Errorf("can't parse container name for annotation %q", a.Key())
	}

	// TODO(ilya-lesikov): validate container name

	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	duration, err := time.ParseDuration(strings.TrimSpace(a.Value()))
	if err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected valid duration", a.Value(), a.Key())
	}

	if math.Signbit(duration.Seconds()) {
		return fmt.Errorf("invalid value %q for annotation %q, expected positive duration value", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationIgnoreReadinessProbeFailsFor) ForContainer() string {
	matches := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(a.Key())
	containerSubexpIndex := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
	return matches[containerSubexpIndex]
}

func (a *AnnotationIgnoreReadinessProbeFailsFor) IgnoreReadinessProbeFailsFor() time.Duration {
	duration, _ := time.ParseDuration(strings.TrimSpace(a.Value()))
	return duration
}
