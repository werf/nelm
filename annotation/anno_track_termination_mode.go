package annotation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var AnnotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

var _ Annotationer = (*AnnotationTrackTerminationMode)(nil)

func NewAnnotationTrackTerminationMode(key, value string) *AnnotationTrackTerminationMode {
	return &AnnotationTrackTerminationMode{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationTrackTerminationMode struct{ *baseAnnotation }

func (a *AnnotationTrackTerminationMode) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	allowedValues := []multitrack.TrackTerminationMode{
		multitrack.WaitUntilResourceReady,
		multitrack.NonBlocking,
	}

	for _, allowedValue := range allowedValues {
		if strings.TrimSpace(a.Value()) == string(allowedValue) {
			return nil
		}
	}

	var stringedAllowedValues []string
	for _, allowedValue := range allowedValues {
		stringedAllowedValues = append(stringedAllowedValues, string(allowedValue))
	}

	return fmt.Errorf("invalid value %q for annotation %q, allowed values: %s", a.Value(), a.Key(), strings.Join(stringedAllowedValues, ", "))
}

func (a *AnnotationTrackTerminationMode) TrackTerminationMode() multitrack.TrackTerminationMode {
	return multitrack.TrackTerminationMode(strings.TrimSpace(a.Value()))
}
