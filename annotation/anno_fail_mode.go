package annotation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var AnnotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)

var _ Annotationer = (*AnnotationFailMode)(nil)

func NewAnnotationFailMode(key, value string) *AnnotationFailMode {
	return &AnnotationFailMode{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationFailMode struct{ *baseAnnotation }

func (a *AnnotationFailMode) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	allowedValues := []multitrack.FailMode{
		multitrack.IgnoreAndContinueDeployProcess,
		multitrack.FailWholeDeployProcessImmediately,
		multitrack.HopeUntilEndOfDeployProcess,
	}

	var validValue bool
	for _, allowedValue := range allowedValues {
		if strings.TrimSpace(a.Value()) == string(allowedValue) {
			validValue = true
			break
		}
	}
	if !validValue {
		var stringedAllowedValues []string
		for _, allowedValue := range allowedValues {
			stringedAllowedValues = append(stringedAllowedValues, string(allowedValue))
		}
		return fmt.Errorf("invalid value %q for annotation %q, allowed values: %s", a.Value(), a.Key(), strings.Join(stringedAllowedValues, ", "))
	}

	return nil
}

func (a *AnnotationFailMode) FailMode() multitrack.FailMode {
	return multitrack.FailMode(strings.TrimSpace(a.Value()))
}
