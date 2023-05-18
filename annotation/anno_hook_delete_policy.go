package annotation

import (
	"fmt"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/werf/common"
)

var AnnotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

var _ Annotationer = (*AnnotationHookDeletePolicy)(nil)

func NewAnnotationHookDeletePolicy(key, value string) *AnnotationHookDeletePolicy {
	return &AnnotationHookDeletePolicy{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationHookDeletePolicy struct{ *baseAnnotation }

func (a *AnnotationHookDeletePolicy) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if strings.Contains(strings.TrimSpace(a.Value()), ",") {
		for _, policy := range strings.Split(strings.TrimSpace(a.Value()), ",") {
			if strings.TrimSpace(policy) == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated hook delete policies has no name", a.Value(), a.Key())
			}

			var validPolicy bool
			for _, deletePolicy := range common.HookDeletePolicies {
				if policy == string(deletePolicy) {
					validPolicy = true
					break
				}
			}
			if !validPolicy {
				return fmt.Errorf("invalid value %q for annotation %q, hook delete policy %q is invalid", a.Value(), a.Key(), policy)
			}
		}
	}

	return nil
}

func (a *AnnotationHookDeletePolicy) HookDeletePolicies() []common.HelmHookDeletePolicy {
	var result []common.HelmHookDeletePolicy
	for _, policy := range strings.Split(strings.TrimSpace(a.Value()), ",") {
		result = append(result, common.HelmHookDeletePolicy(policy))
	}
	if result == nil {
		result = []common.HelmHookDeletePolicy{common.DefaultHookPolicy}
	}

	return result
}
