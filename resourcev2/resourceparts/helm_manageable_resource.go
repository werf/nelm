package resourceparts

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)
var annotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)
var labelKeyManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

func NewHelmManageableResource(unstruct *unstructured.Unstructured) *HelmManageableResource {
	return &HelmManageableResource{unstructured: unstruct}
}

type HelmManageableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *HelmManageableResource) Validate() error {
	return nil
}

func (r *HelmManageableResource) OwnableByRelease(releaseName, releaseNamespace string) (ownable bool, nonOwnableReasons []string) {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, key, value))
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, key, value))
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetLabels(), labelKeyManagedBy); found {
		if value != "Helm" {
			nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`label "%s=%s" must have value "Helm"`, key, value))
		}
	} else {
		nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`label %q not found, must be set to "Helm"`, key))
	}

	return len(nonOwnableReasons) == 0, nonOwnableReasons
}
