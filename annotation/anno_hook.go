package annotation

import (
	"fmt"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/werf/common"
)

var AnnotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

var _ Annotationer = (*AnnotationHook)(nil)

func NewAnnotationHook(key, value string) *AnnotationHook {
	return &AnnotationHook{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationHook struct{ *baseAnnotation }

func (a *AnnotationHook) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if strings.Contains(strings.TrimSpace(a.Value()), ",") {
		for _, hookType := range strings.Split(strings.TrimSpace(a.Value()), ",") {
			if strings.TrimSpace(hookType) == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated hook types has no name", a.Value(), a.Key())
			}

			for _, allowedHookType := range common.HelmHookTypes {
				if hookType == string(allowedHookType) {
					return nil
				}
			}

			return fmt.Errorf("invalid value %q for annotation %q, hook type %q is invalid", a.Value(), a.Key(), hookType)
		}
	}

	return nil
}

func (a *AnnotationHook) HookTypes() []common.HelmHookType {
	var result []common.HelmHookType
	for _, hookType := range strings.Split(strings.TrimSpace(a.Value()), ",") {
		result = append(result, common.HelmHookType(hookType))
	}
	return result
}
