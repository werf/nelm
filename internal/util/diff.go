package util

import (
	"regexp"
	"strings"

	"github.com/aymanbagabas/go-udiff"
	"github.com/aymanbagabas/go-udiff/myers"
	"github.com/gookit/color"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ColoredUnifiedDiff(from, to string) (uDiff string, present bool) {
	edits := myers.ComputeEdits(from, to)
	if len(edits) == 0 {
		return "", false
	}

	uncoloredUDiff := lo.Must1(udiff.ToUnified("", "", from, edits, udiff.DefaultContextLines))

	var uDiffLines []string
	var firstHunkHeaderStripped bool
	lines := strings.Split(uncoloredUDiff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || (i == len(lines)-1 && strings.TrimSpace(line) == "") {
			continue
		}

		if strings.HasPrefix(line, "@@ ") && strings.HasSuffix(line, " @@") {
			if !firstHunkHeaderStripped {
				firstHunkHeaderStripped = true
				continue
			}
			uDiffLines = append(uDiffLines, color.Gray.Renderln("   ..."))
		} else if strings.HasPrefix(line, "+") {
			uDiffLines = append(uDiffLines, color.Green.Renderln(line[:1]+" "+line[1:]))
		} else if strings.HasPrefix(line, "-") {
			uDiffLines = append(uDiffLines, color.Red.Renderln(line[:1]+" "+line[1:]))
		} else if strings.TrimSpace(line) == "" {
			uDiffLines = append(uDiffLines, color.Gray.Renderln(line))
		} else {
			uDiffLines = append(uDiffLines, color.Gray.Renderln(" "+line))
		}
	}

	if len(uDiffLines) == 0 {
		return "", false
	}

	return strings.Trim(strings.Join(uDiffLines, "\n"), "\n"), true
}

type BuildDiffableUnstructsOptions struct {
	CleanAnnotationsRegexes []*regexp.Regexp
	CleanLabelsRegexes      []*regexp.Regexp
	NoCleanRuntimeData      bool
}

func BuildDiffableUnstructs(unstruct1, unstruct2 *unstructured.Unstructured, opts BuildDiffableUnstructsOptions) (*unstructured.Unstructured, *unstructured.Unstructured) {
	unstructCopy1 := unstruct1.DeepCopy()
	unstructCopy2 := unstruct2.DeepCopy()

	if !opts.NoCleanRuntimeData {
		cleanRuntimeDataFromUnstruct(unstructCopy1)
		cleanRuntimeDataFromUnstruct(unstructCopy2)
	}

	filteredAnnos1 := filterAnnosOrLabels(unstructCopy1.GetAnnotations(), opts.CleanAnnotationsRegexes)
	unstructCopy1.SetAnnotations(filteredAnnos1)

	filteredAnnos2 := filterAnnosOrLabels(unstructCopy2.GetAnnotations(), opts.CleanAnnotationsRegexes)
	unstructCopy2.SetAnnotations(filteredAnnos2)

	filteredLabels1 := filterAnnosOrLabels(unstructCopy1.GetLabels(), opts.CleanLabelsRegexes)
	unstructCopy1.SetLabels(filteredLabels1)

	filteredLabels2 := filterAnnosOrLabels(unstructCopy2.GetLabels(), opts.CleanLabelsRegexes)
	unstructCopy2.SetLabels(filteredLabels2)

	return unstructCopy1, unstructCopy2
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
	unstruct.SetCreationTimestamp(metav1.Time{})
	unstruct.Object["status"] = nil

	managedFields := unstruct.GetManagedFields()
	for _, entry := range managedFields {
		entry.Time = nil
	}
	unstruct.SetManagedFields(managedFields)
}
