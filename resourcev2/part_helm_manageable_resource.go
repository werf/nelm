package resourcev2

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var annotationKeyHumanReleaseName = "meta.helm.sh/release-name"
var annotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

var annotationKeyHumanReleaseNamespace = "meta.helm.sh/release-namespace"
var annotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

var labelKeyHumanManagedBy = "app.kubernetes.io/managed-by"
var labelKeyPatternManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

func newHelmManageableResource(unstruct *unstructured.Unstructured) *helmManageableResource {
	return &helmManageableResource{unstructured: unstruct}
}

type helmManageableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *helmManageableResource) Validate() error {
	return nil
}

func (r *helmManageableResource) OwnableByRelease(releaseName, releaseNamespace string) (ownable bool, nonOwnableReason string) {
	nonOwnableReasons := []string{}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, annotationKeyHumanReleaseName, value))
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonOwnableReasons = append(nonOwnableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, annotationKeyHumanReleaseNamespace, value))
	}

	nonOwnableReason = strings.Join(nonOwnableReasons, ", ")

	return len(nonOwnableReasons) == 0, nonOwnableReason
}
