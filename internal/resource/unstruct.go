package resource

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/common"
)

type CleanUnstructOptions struct {
	CleanHelmShAnnos        bool
	CleanManagedFields      bool
	CleanReleaseAnnosLabels bool
	CleanRuntimeData        bool
	CleanWerfIoAnnos        bool
	CleanWerfIoRuntimeAnnos bool
}

func CleanUnstruct(unstruct *unstructured.Unstructured, opts CleanUnstructOptions) *unstructured.Unstructured {
	unstructCopy := unstruct.DeepCopy()

	if opts.CleanRuntimeData {
		cleanRuntimeDataFromUnstruct(unstructCopy)
	}

	if opts.CleanManagedFields {
		unstructCopy.SetManagedFields(nil)
	}

	var (
		cleanAnnotationsRegexes []*regexp.Regexp
		cleanLabelsRegexes      []*regexp.Regexp
	)

	if opts.CleanHelmShAnnos {
		cleanAnnotationsRegexes = append(cleanAnnotationsRegexes, regexp.MustCompile(`^helm\.sh/.+`))
	}

	if opts.CleanWerfIoAnnos {
		cleanAnnotationsRegexes = append(cleanAnnotationsRegexes, regexp.MustCompile(`.*werf\.io/.+`))
	}

	if opts.CleanWerfIoRuntimeAnnos {
		cleanAnnotationsRegexes = append(cleanAnnotationsRegexes,
			regexp.MustCompile(`.*ci\.werf\.io/.+`),
			regexp.MustCompile(`^project\.werf\.io/.+`),
			regexp.MustCompile(`^werf\.io/version$`), regexp.MustCompile(`^werf\.io/release-channel$`),
		)
	}

	if opts.CleanReleaseAnnosLabels {
		cleanAnnotationsRegexes = append(cleanAnnotationsRegexes, common.AnnotationKeyPatternReleaseName, common.AnnotationKeyPatternReleaseNamespace)
		cleanLabelsRegexes = append(cleanLabelsRegexes, common.LabelKeyPatternManagedBy)
	}

	if annos := unstructCopy.GetAnnotations(); len(annos) > 0 {
		filteredAnnos := filterAnnosOrLabels(annos, cleanAnnotationsRegexes)
		unstructCopy.SetAnnotations(filteredAnnos)
	}

	if labels := unstructCopy.GetLabels(); len(labels) > 0 {
		filteredLabels := filterAnnosOrLabels(labels, cleanLabelsRegexes)
		unstructCopy.SetLabels(filteredLabels)
	}

	return unstructCopy
}

func filterAnnosOrLabels(annosOrLabels map[string]string, regexes []*regexp.Regexp) map[string]string {
	filtered := map[string]string{}

annoOrLabelLoop:
	for key, val := range annosOrLabels {
		for _, regex := range regexes {
			if regex.MatchString(key) {
				continue annoOrLabelLoop
			}
		}

		filtered[key] = val
	}

	return filtered
}

func cleanRuntimeDataFromUnstruct(unstruct *unstructured.Unstructured) {
	unstruct.SetResourceVersion("")
	unstruct.SetGeneration(0)
	unstruct.SetUID("")
	unstruct.SetCreationTimestamp(v1.Time{})
	unstruct.SetSelfLink("")
	unstruct.SetFinalizers(nil)
	delete(unstruct.Object, "status")

	managedFields := unstruct.GetManagedFields()
	for _, entry := range managedFields {
		entry.Time = nil
	}

	unstruct.SetManagedFields(managedFields)
}
