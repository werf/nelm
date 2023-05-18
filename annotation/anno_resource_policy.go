package annotation

import (
	"fmt"
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/werf/common"
)

var AnnotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

var _ Annotationer = (*AnnotationResourcePolicy)(nil)

func NewAnnotationResourcePolicy(key, value string) *AnnotationResourcePolicy {
	return &AnnotationResourcePolicy{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationResourcePolicy struct{ *baseAnnotation }

func (a *AnnotationResourcePolicy) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	var validResourcePolicy bool
	for _, policy := range common.ResourcePolicies {
		if strings.TrimSpace(a.Value()) == string(policy) {
			validResourcePolicy = true
			break
		}
	}
	if !validResourcePolicy {
		return fmt.Errorf("invalid value %q for annotation %q, value must be one of %v", a.Value(), a.Key(), common.ResourcePolicies)
	}

	return nil
}

func (a *AnnotationResourcePolicy) ResourcePolicy() common.ResourcePolicy {
	if resourcePolicy := common.ResourcePolicy(strings.TrimSpace(a.Value())); resourcePolicy == "" {
		return common.DefaultResourcePolicy
	} else {
		return resourcePolicy
	}
}
