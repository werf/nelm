package resourcev2

import (
	"regexp"
	"strings"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func IsCRD(res CRDDetectable) bool {
	return res.GroupVersionKind().GroupKind() == schema.GroupKind{
		Group: "apiextensions.k8s.io",
		Kind:  "CustomResourceDefinition",
	}
}

func EqualNameNamespaceGVK(res1, res2 EquatableByNameNamespaceGVK) bool {
	return res1.Name() == res2.Name() && res1.Namespace() == res2.Namespace() && res1.GroupVersionKind() == res2.GroupVersionKind()
}

func IsHook(annotations map[string]string) bool {
	_, _, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternHook)
	return found
}

func FindAnnotationOrLabelByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (key string, value string, found bool) {
	key, found = lo.FindKeyBy(annotationsOrLabels, func(k string, _ string) bool {
		return pattern.MatchString(k)
	})
	if found {
		value = strings.TrimSpace(annotationsOrLabels[key])
	}

	return key, value, found
}

func FindAnnotationsOrLabelsByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (result map[string]string, found bool) {
	result = map[string]string{}

	for key, value := range annotationsOrLabels {
		if pattern.MatchString(key) {
			result[key] = strings.TrimSpace(value)
		}
	}

	return result, len(result) > 0
}

func validateDeletePolicyAnnotations(annotations map[string]string) error {
	if IsHook(annotations) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternHookDeletePolicy); found {
			if value == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
			}

			for _, hookDeletePolicy := range strings.Split(value, ",") {
				hookDeletePolicy = strings.TrimSpace(hookDeletePolicy)
				if hookDeletePolicy == "" {
					return errors.NewValidationError("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}

				switch hookDeletePolicy {
				case string(release.HookSucceeded),
					string(release.HookFailed),
					string(release.HookBeforeHookCreation):
				default:
					return errors.NewValidationError("value %q for annotation %q is not supported", value, key, hookDeletePolicy)
				}
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternDeletePolicy); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		for _, deletePolicy := range strings.Split(value, ",") {
			deletePolicy = strings.TrimSpace(deletePolicy)
			if deletePolicy == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch deletePolicy {
			case string(common.DeletePolicySucceeded),
				string(common.DeletePolicyFailed),
				string(common.DeletePolicyBeforeCreation):
			default:
				return errors.NewValidationError("value %q for annotation %q is not supported", value, key, deletePolicy)
			}
		}
	}

	return nil
}

func getDeletePolicies(annotations map[string]string) []common.DeletePolicy {
	var deletePolicies []common.DeletePolicy
	if IsHook(annotations) {
		hookDeletePoliciesValues, hookDeletePoliciesFound := FindAnnotationsOrLabelsByKeyPattern(annotations, annotationKeyPatternHookDeletePolicy)

		generalDeletePoliciesValues, generalDeletePoliciesFound := FindAnnotationsOrLabelsByKeyPattern(annotations, annotationKeyPatternDeletePolicy)

		if !hookDeletePoliciesFound && !generalDeletePoliciesFound {
			deletePolicies = append(deletePolicies, common.DeletePolicyBeforeCreation)
		} else if generalDeletePoliciesFound {
			for _, generalDeletePolicyValue := range generalDeletePoliciesValues {
				deletePolicies = append(deletePolicies, common.DeletePolicy(generalDeletePolicyValue))
			}
		} else {
			for _, hookDeletePolicyValue := range hookDeletePoliciesValues {
				switch hookDeletePolicyValue {
				case string(release.HookSucceeded):
					deletePolicies = append(deletePolicies, common.DeletePolicySucceeded)
				case string(release.HookFailed):
					deletePolicies = append(deletePolicies, common.DeletePolicyFailed)
				case string(release.HookBeforeHookCreation):
					deletePolicies = append(deletePolicies, common.DeletePolicyBeforeCreation)
				}
			}
		}
	} else {
		deletePoliciesValues, deletePoliciesFound := FindAnnotationsOrLabelsByKeyPattern(annotations, annotationKeyPatternDeletePolicy)
		if deletePoliciesFound {
			for _, deletePolicyValue := range deletePoliciesValues {
				deletePolicies = append(deletePolicies, common.DeletePolicy(deletePolicyValue))
			}
		}
	}

	return deletePolicies
}

type CRDDetectable interface {
	GroupVersionKind() schema.GroupVersionKind
}

type EquatableByNameNamespaceGVK interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
}
