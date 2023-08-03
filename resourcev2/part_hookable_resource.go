package resourcev2

import (
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyHumanHook = "helm.sh/hook"
var annotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

func newHookableResource(unstruct *unstructured.Unstructured) *hookableResource {
	return &hookableResource{unstructured: unstruct}
}

type hookableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *hookableResource) Validate() error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		for _, hookType := range strings.Split(value, ",") {
			hookType = strings.TrimSpace(hookType)
			if hookType == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch hookType {
			case string(release.HookPreInstall),
				string(release.HookPostInstall),
				string(release.HookPreUpgrade),
				string(release.HookPostUpgrade),
				string(release.HookPreRollback),
				string(release.HookPostRollback),
				string(release.HookPreDelete),
				string(release.HookPostDelete),
				string(release.HookTest),
				"test-success":
			default:
				return errors.NewValidationError("value %q for annotation %q is not supported", value, key, hookType)
			}
		}
	} else {
		return errors.NewValidationError(`hook resource must have %q annotation`, annotationKeyHumanHook)
	}

	return nil
}

func (r *hookableResource) HookPreInstall() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreInstall)
}

func (r *hookableResource) HookPostInstall() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostInstall)
}

func (r *hookableResource) HookPreUpgrade() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreUpgrade)
}

func (r *hookableResource) HookPostUpgrade() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostUpgrade)
}

func (r *hookableResource) HookPreRollback() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreRollback)
}

func (r *hookableResource) HookPostRollback() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostRollback)
}

func (r *hookableResource) HookPreDelete() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreDelete)
}

func (r *hookableResource) HookPostDelete() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostDelete)
}

func (r *hookableResource) HookTest() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookTest) || value == "test-success"
}
