package resourceparts

import (
	"regexp"
	"strings"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)
var annotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

func NewRecreatableResource(unstruct *unstructured.Unstructured) *RecreatableResource {
	return &RecreatableResource{unstructured: unstruct}
}

type RecreatableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *RecreatableResource) Validate() error {
	if err := validateDeletePolicyAnnotations(r.unstructured.GetAnnotations()); err != nil {
		return err
	}

	return nil
}

func (r *RecreatableResource) ShouldRecreate() bool {
	deletePolicies := getDeletePolicies(r.unstructured.GetAnnotations())

	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreation)
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
