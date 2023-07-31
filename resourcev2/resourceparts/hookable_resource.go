package resourceparts

import (
	"regexp"
	"strings"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

func NewHookableResource(unstruct *unstructured.Unstructured) *HookableResource {
	return &HookableResource{unstructured: unstruct}
}

type HookableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *HookableResource) Validate() error {
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
		return errors.NewValidationError(`hook resource must have "helm.sh/hook" annotation`)
	}

	return nil
}

func (r *HookableResource) HookPreInstall() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreInstall)
}

func (r *HookableResource) HookPostInstall() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostInstall)
}

func (r *HookableResource) HookPreUpgrade() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreUpgrade)
}

func (r *HookableResource) HookPostUpgrade() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostUpgrade)
}

func (r *HookableResource) HookPreRollback() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreRollback)
}

func (r *HookableResource) HookPostRollback() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostRollback)
}

func (r *HookableResource) HookPreDelete() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPreDelete)
}

func (r *HookableResource) HookPostDelete() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookPostDelete)
}

func (r *HookableResource) HookTest() bool {
	_, value, _ := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternHook)
	return value == string(release.HookTest) || value == "test-success"
}
