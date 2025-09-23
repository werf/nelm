package resource

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CleanUnstructOptions struct {
	CleanHelmShAnnos        bool
	CleanManagedFiles       bool
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

	if opts.CleanManagedFiles {
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
			regexp.MustCompile(`^project\.werf\.io/.+`),
			regexp.MustCompile(`^ci\.werf\.io/.+`),
			regexp.MustCompile(`^werf\.io/version$`), regexp.MustCompile(`^werf\.io/release-channel$`),
		)
	}

	if opts.CleanReleaseAnnosLabels {
		cleanAnnotationsRegexes = append(cleanAnnotationsRegexes, AnnotationKeyPatternReleaseName, AnnotationKeyPatternReleaseNamespace)
		cleanLabelsRegexes = append(cleanLabelsRegexes, LabelKeyPatternManagedBy)
	}

	filteredAnnos := filterAnnosOrLabels(unstructCopy.GetAnnotations(), cleanAnnotationsRegexes)
	unstructCopy.SetAnnotations(filteredAnnos)

	filteredLabels := filterAnnosOrLabels(unstructCopy.GetLabels(), cleanLabelsRegexes)
	unstructCopy.SetLabels(filteredLabels)

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
	unstruct.Object["status"] = nil

	managedFields := unstruct.GetManagedFields()
	for _, entry := range managedFields {
		entry.Time = nil
	}

	unstruct.SetManagedFields(managedFields)
}
